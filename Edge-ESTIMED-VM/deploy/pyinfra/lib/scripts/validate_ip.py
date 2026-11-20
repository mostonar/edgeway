# Pre-flight check for MEC_HOST_ADDRESS
#!/usr/bin/env python3
import socket
import ipaddress
import sys

if len(sys.argv) < 2:
    sys.exit(0)
    
ip = sys.argv[1]

try:
    ipaddress.ip_address(ip)
    s = socket.socket(socket.AF_INET, socket.SOCK_DGRAM)
    s.bind((ip, 0))
    s.close()
except ValueError:
    # If it's not a valid IP (e.g. it's a domain name), skip the check
    sys.exit(0)
except OSError:
    print(f"\nERROR: MEC_HOST_ADDRESS '{ip}' does not match any of this remote machine's actual IP addresses. Did the IP change? Please check .env MEC_HOST_ADDRESS and try again.\n", file=sys.stderr)
    sys.exit(1)
