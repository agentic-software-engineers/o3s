package main

import (
	"bufio"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"os"
	"regexp"
	"strings"
	"time"
)

// loadAllow reads one regular expression per line, the names the proxy lets through
func loadAllow(path string) []*regexp.Regexp {
	b, err := os.ReadFile(path)
	if err != nil {
		log.Fatal(err)
	}
	var out []*regexp.Regexp
	for _, l := range strings.Fields(string(b)) {
		out = append(out, regexp.MustCompile(l))
	}
	return out
}

func allowed(host string, allow []*regexp.Regexp) bool {
	for _, re := range allow {
		if re.MatchString(host) {
			return true
		}
	}
	return false
}

// serveProxy answers one client: CONNECT to an allowed name on 443, or a plain HTTP request to an allowed name, and refuses the rest
func serveProxy(c net.Conn, allow []*regexp.Regexp, logf io.Writer) {
	defer c.Close()
	br := bufio.NewReader(c)
	req, err := http.ReadRequest(br)
	if err != nil {
		return
	}
	host, port := req.URL.Hostname(), req.URL.Port()
	if req.Method == http.MethodConnect {
		host, port, _ = net.SplitHostPort(req.Host)
	} else if port == "" {
		port = "80"
	}
	if !allowed(host, allow) || (req.Method == http.MethodConnect && port != "443") {
		fmt.Fprintf(logf, "%s refused %s %s:%s\n", time.Now().Format(time.RFC3339), req.Method, host, port)
		_, _ = io.WriteString(c, "HTTP/1.1 403 Forbidden\r\nContent-Length: 0\r\nConnection: close\r\n\r\n")
		return
	}
	up, err := net.DialTimeout("tcp", net.JoinHostPort(host, port), 20*time.Second)
	if err != nil {
		_, _ = io.WriteString(c, "HTTP/1.1 502 Bad Gateway\r\nContent-Length: 0\r\nConnection: close\r\n\r\n")
		return
	}
	defer up.Close()
	if req.Method == http.MethodConnect {
		_, _ = io.WriteString(c, "HTTP/1.1 200 Connection established\r\n\r\n")
	} else {
		req.RequestURI = ""
		req.Close = true
		req.Header.Del("Proxy-Connection")
		if err := req.Write(up); err != nil {
			return
		}
	}
	done := make(chan struct{})
	go func() { _, _ = io.Copy(up, br); close(done) }()
	_, _ = io.Copy(c, up)
	<-done
}

// proxyUnix serves the allow-list proxy on the Unix socket path, which vfkit dials for each guest connection
func proxyUnix(path, allowFile, logFile string) {
	allow := loadAllow(allowFile)
	logf, err := os.OpenFile(logFile, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o600)
	if err != nil {
		log.Fatal(err)
	}
	_ = os.Remove(path)
	ln, err := net.Listen("unix", path)
	if err != nil {
		log.Fatal(err)
	}
	for {
		c, err := ln.Accept()
		if err != nil {
			log.Fatal(err)
		}
		go serveProxy(c, allow, logf)
	}
}
