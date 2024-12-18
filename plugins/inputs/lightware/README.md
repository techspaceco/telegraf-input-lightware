# Lightware Input Plugin

Gathers statistics about Lightware products using LW3.

### Configuration:

```toml
# Read metrics about dnsmasq dns side.
[[inputs.lightware]]
  # IP or hostname of the Lightware LW3 device.
  urls = [
    "https://user:pass@localhost:443",
  ]

  # Paths.
  # LW3 API paths to query. Paths are lowercased and / replaced with _.
  #
  # Default:
  paths = [
    "/PackageVersion",
    "/V1/MEDIA/USB/U1/Connected",
    "/V1/MEDIA/OCS/P1/State",
    "/V1/SYS/DEVICES/RX/Connected",
    "/V1/SYS/DEVICES/RX/PackageVersion",
  ]
```

### Metrics:

- lightware
  - tags:
    - host # hostname from url
    - label # /V1/MANAGEMENT/LABEL
    - product_name # /ProductName
  - fields:
    - package_version
    - v1_media_usb_u1_connected
    - v1_media_ocs_p1_state
    - v1_sys_devices_rx_connected
    - v1_sys_devices_rx_package_version

### Example Output:

```
```