//go:build windows

// The Windows side: boot the cage VM through the Host Compute Service with no network adapter,
// and link it to the host over Hyper-V sockets alone.
//
// The HCS bindings follow lima-vm/lima pkg/driver/hcs/hcs_api_windows.go (Apache-2.0) at v2.3.0.
package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"time"
	"unsafe"

	"github.com/Microsoft/go-winio"
	"github.com/Microsoft/go-winio/pkg/guid"
	"golang.org/x/sys/windows"
)

var (
	computecore      = windows.NewLazySystemDLL("computecore.dll")
	procCreateOp     = computecore.NewProc("HcsCreateOperation")
	procCloseOp      = computecore.NewProc("HcsCloseOperation")
	procWaitOp       = computecore.NewProc("HcsWaitForOperationResult")
	procCreateSystem = computecore.NewProc("HcsCreateComputeSystem")
	procStartSystem  = computecore.NewProc("HcsStartComputeSystem")
	procGrantAccess  = computecore.NewProc("HcsGrantVmAccess")
	procProperties   = computecore.NewProc("HcsGetComputeSystemProperties")
)

const (
	vmName    = "o3s-cage"
	proxyPort = 3128
	pipeName  = `\\.\pipe\o3s-cage-com1`
)

// hresult turns an HRESULT into an error, unwrapping FACILITY_WIN32 as hcsshim does
func hresult(r0 uintptr) error {
	if int32(r0) >= 0 {
		return nil
	}
	if r0&0x1fff0000 == 0x00070000 {
		r0 &= 0xffff
	}
	return syscall.Errno(r0)
}

// call runs one asynchronous HCS call through a fresh operation and returns its result document
func call(what string, f func(op uintptr) uintptr) (string, error) {
	op, _, e := syscall.SyscallN(procCreateOp.Addr(), 0, 0)
	if op == 0 {
		return "", fmt.Errorf("HcsCreateOperation: %w", e)
	}
	defer syscall.SyscallN(procCloseOp.Addr(), op)
	if err := hresult(f(op)); err != nil {
		return "", fmt.Errorf("%s: %w", what, err)
	}
	var result *uint16
	r0, _, _ := syscall.SyscallN(procWaitOp.Addr(), op, 0xFFFFFFFF, uintptr(unsafe.Pointer(&result)))
	doc := ""
	if result != nil {
		doc = windows.UTF16PtrToString(result)
		windows.CoTaskMemFree(unsafe.Pointer(result))
	}
	if err := hresult(r0); err != nil {
		return doc, fmt.Errorf("%s: %w (result: %s)", what, err, doc)
	}
	return doc, nil
}

func utf16(s string) *uint16 {
	p, err := syscall.UTF16PtrFromString(s)
	if err != nil {
		log.Fatal(err)
	}
	return p
}

// config is the compute system document: UEFI boot from the disk, the seed, a serial port, Hyper-V sockets, and no network adapter
func config(disk, iso string) string {
	service := map[string]any{
		"AllowWildcardBinds":        true,
		"BindSecurityDescriptor":    "D:P(A;;FA;;;WD)",
		"ConnectSecurityDescriptor": "D:P(A;;FA;;;SY)(A;;FA;;;BA)",
	}
	doc := map[string]any{
		"SchemaVersion":                     map[string]any{"Major": 2, "Minor": 1},
		"Owner":                             "o3s-spike",
		"ShouldTerminateOnLastHandleClosed": true,
		"VirtualMachine": map[string]any{
			"Chipset": map[string]any{"Uefi": map[string]any{
				"Console":  "ComPort1",
				"BootThis": map[string]any{"DevicePath": "Primary disk", "DiskNumber": 0, "DeviceType": "ScsiDrive"},
			}},
			"ComputeTopology": map[string]any{
				"Memory":    map[string]any{"Backing": "Virtual", "SizeInMB": 6144},
				"Processor": map[string]any{"Count": 2},
			},
			"Devices": map[string]any{
				"Scsi": map[string]any{
					"Primary disk": map[string]any{"Attachments": map[string]any{"0": map[string]any{"Type": "VirtualDisk", "Path": disk}}},
					"cidata":       map[string]any{"Attachments": map[string]any{"0": map[string]any{"Type": "Iso", "Path": iso}}},
				},
				"ComPorts": map[string]any{"0": map[string]any{"NamedPipe": pipeName}},
				"HvSocket": map[string]any{"HvSocketConfig": map[string]any{
					"DefaultBindSecurityDescriptor":    "D:P(A;;FA;;;WD)",
					"DefaultConnectSecurityDescriptor": "D:P(A;;FA;;;SY)(A;;FA;;;BA)",
					"ServiceTable": map[string]any{
						winio.VsockServiceID(proxyPort).String(): service,
						winio.VsockServiceID(2222).String():      service,
					},
				}},
			},
		},
	}
	b, err := json.Marshal(doc)
	if err != nil {
		log.Fatal(err)
	}
	return string(b)
}

func run(dir string) {
	disk, iso := filepath.Join(dir, "disk.vhdx"), filepath.Join(dir, "seed.iso")
	allow := loadAllow(filepath.Join(dir, "allow.txt"))
	proxyLog, err := os.OpenFile(filepath.Join(dir, "proxy.log"), os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o600)
	if err != nil {
		log.Fatal(err)
	}
	// Copy the guest's serial console into console.log
	serial, err := winio.ListenPipe(pipeName, nil)
	if err != nil {
		log.Fatal(err)
	}
	go func() {
		f, _ := os.Create(filepath.Join(dir, "console.log"))
		for {
			c, err := serial.Accept()
			if err != nil {
				return
			}
			_, _ = io.Copy(f, c)
			c.Close()
		}
	}()
	// Let the VM's worker process open the disk and the seed
	for _, p := range []string{disk, iso} {
		if err := hresult(func() uintptr {
			r, _, _ := syscall.SyscallN(procGrantAccess.Addr(), uintptr(unsafe.Pointer(utf16(vmName))), uintptr(unsafe.Pointer(utf16(p))))
			return r
		}()); err != nil {
			log.Fatalf("grant %s: %v", p, err)
		}
	}
	var system uintptr
	if _, err := call("HcsCreateComputeSystem", func(op uintptr) uintptr {
		r, _, _ := syscall.SyscallN(procCreateSystem.Addr(), uintptr(unsafe.Pointer(utf16(vmName))), uintptr(unsafe.Pointer(utf16(config(disk, iso)))), op, 0, uintptr(unsafe.Pointer(&system)))
		return r
	}); err != nil {
		log.Fatal(err)
	}
	if _, err := call("HcsStartComputeSystem", func(op uintptr) uintptr {
		r, _, _ := syscall.SyscallN(procStartSystem.Addr(), system, op, 0)
		return r
	}); err != nil {
		log.Fatal(err)
	}
	// Read the runtime ID, the VM GUID Hyper-V sockets address
	doc, err := call("HcsGetComputeSystemProperties", func(op uintptr) uintptr {
		r, _, _ := syscall.SyscallN(procProperties.Addr(), system, op, uintptr(unsafe.Pointer(utf16("{}"))))
		return r
	})
	if err != nil {
		log.Fatal(err)
	}
	var props struct{ RuntimeId string }
	if err := json.Unmarshal([]byte(doc), &props); err != nil || props.RuntimeId == "" {
		log.Fatalf("no RuntimeId in %s: %v", doc, err)
	}
	vmid, err := guid.FromString(props.RuntimeId)
	if err != nil {
		log.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "vmid"), []byte(vmid.String()), 0o600); err != nil {
		log.Fatal(err)
	}
	log.Printf("the cage VM runs as %s with no network adapter", vmid)
	// Serve the allow-list proxy to the guest's vsock port 3128
	ln, err := winio.ListenHvsock(&winio.HvsockAddr{VMID: vmid, ServiceID: winio.VsockServiceID(proxyPort)})
	if err != nil {
		log.Fatal(err)
	}
	for {
		c, err := ln.Accept()
		if err != nil {
			log.Fatal(err)
		}
		go serveProxy(c, allow, proxyLog)
	}
}

func stdio(dir string, port uint32) {
	b, err := os.ReadFile(filepath.Join(dir, "vmid"))
	if err != nil {
		log.Fatal(err)
	}
	vmid, err := guid.FromString(strings.TrimSpace(string(b)))
	if err != nil {
		log.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	c, err := winio.Dial(ctx, &winio.HvsockAddr{VMID: vmid, ServiceID: winio.VsockServiceID(port)})
	if err != nil {
		log.Fatal(err)
	}
	go func() { _, _ = io.Copy(c, os.Stdin); _ = c.CloseWrite() }()
	_, _ = io.Copy(os.Stdout, c)
}

// platform runs the commands only Windows offers
func platform(args []string) {
	switch args[0] {
	case "run":
		run(args[1])
	case "stdio":
		port, err := strconv.ParseUint(args[2], 10, 32)
		if err != nil {
			log.Fatal(err)
		}
		stdio(args[1], uint32(port))
	default:
		log.Fatalf("unknown command %q", args[0])
	}
}
