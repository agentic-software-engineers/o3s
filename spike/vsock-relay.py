"""Relay a byte stream between a TCP socket and a vsock socket (AF_VSOCK, Python socket docs).

host PORT TARGET: listen on vsock PORT for any CID, and carry each connection to TCP 127.0.0.1:TARGET.
guest PORT: listen on TCP PORT and carry each connection to vsock PORT on the host, CID 2.
stdio CID PORT: carry stdin and stdout to vsock PORT on CID, for an SSH ProxyCommand.
unix PATH TARGET: listen on the Unix socket PATH and carry each connection to TCP 127.0.0.1:TARGET (vfkit's listen mode dials PATH).
stdio-unix PATH: carry stdin and stdout to the Unix socket PATH (vfkit's connect mode serves PATH).
"""
import os
import socket
import sys
import threading


def pipe(src, dst):
    """Copy src into dst until src ends, then close both directions."""
    try:
        while data := src.recv(65536):
            dst.sendall(data)
    except OSError:
        pass
    finally:
        # Close both sockets' directions so the other pipe ends too
        for s in (src, dst):
            try:
                s.shutdown(socket.SHUT_RDWR)
            except OSError:
                pass


def serve(listener, dial):
    """Accept connections forever and pipe each to a fresh socket from dial."""
    listener.listen(128)
    # Pipe each accepted connection both ways
    while True:
        client, _ = listener.accept()
        try:
            upstream = dial()
        except OSError:
            client.close()
            continue
        threading.Thread(target=pipe, args=(client, upstream), daemon=True).start()
        threading.Thread(target=pipe, args=(upstream, client), daemon=True).start()


def main():
    mode = sys.argv[1]
    # Pick the side this relay runs on
    if mode == "host":
        port, target = int(sys.argv[2]), int(sys.argv[3])
        listener = socket.socket(socket.AF_VSOCK, socket.SOCK_STREAM)
        listener.bind((socket.VMADDR_CID_ANY, port))
        serve(listener, lambda: socket.create_connection(("127.0.0.1", target)))
    elif mode == "guest":
        port = int(sys.argv[2])
        listener = socket.socket(socket.AF_INET, socket.SOCK_STREAM)
        listener.setsockopt(socket.SOL_SOCKET, socket.SO_REUSEADDR, 1)
        listener.bind(("0.0.0.0", port))

        def dial():
            s = socket.socket(socket.AF_VSOCK, socket.SOCK_STREAM)
            s.connect((socket.VMADDR_CID_HOST, port))
            return s

        serve(listener, dial)
    elif mode == "unix":
        path, target = sys.argv[2], int(sys.argv[3])
        if os.path.exists(path):
            os.unlink(path)
        listener = socket.socket(socket.AF_UNIX, socket.SOCK_STREAM)
        listener.bind(path)
        serve(listener, lambda: socket.create_connection(("127.0.0.1", target)))
    else:
        # Dial the guest, over vsock or through vfkit's Unix socket
        if mode == "stdio-unix":
            s = socket.socket(socket.AF_UNIX, socket.SOCK_STREAM)
            s.connect(sys.argv[2])
        else:
            s = socket.socket(socket.AF_VSOCK, socket.SOCK_STREAM)
            s.connect((int(sys.argv[2]), int(sys.argv[3])))
        reader = threading.Thread(target=lambda: _copy_out(s), daemon=True)
        reader.start()
        # Send stdin until it ends
        while data := sys.stdin.buffer.read1(65536):
            s.sendall(data)
        s.shutdown(socket.SHUT_WR)
        reader.join()


def _copy_out(s):
    """Copy the vsock socket to stdout until it ends."""
    # Write each chunk as it arrives
    while data := s.recv(65536):
        sys.stdout.buffer.write(data)
        sys.stdout.buffer.flush()


main()
