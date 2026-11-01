# Vision compatibility copy

Client, framing and Vision code from SagerNet/sing-vmess v0.2.7, GPL-3.0-or-later.
Upstream: https://github.com/SagerNet/sing-vmess/tree/v0.2.7/vless

Local change: carry unsafe.Pointer through TLS discovery callbacks and use
unsafe.Add for offsets, rather than converting pointers to uintptr and back.
This preserves GC tracking and permits checkptr/race instrumentation. The wire
protocol is unchanged. Retain this patch when refreshing upstream code.

Only client files are included. Root VMess/XUDP helpers remain a pinned external
dependency. Built with with_utls, the uTLS recognition hook is enabled too.
