# MN-CSE WebSocket access

Set `ENABLE_WS=true` in the MN-CSE application's scenario `environment` to
enable the oneM2M WebSocket binding. It is disabled by default. The edgeway
scenario enables it for all three MN-CSE instances.

When enabled, the chart exposes TCP port 8180 on the existing ClusterIP Service
and creates a separate Ingress at the HTTP endpoint's `/ws` suffix:

```text
wss://<host>/<sandbox>/<location>/meep-acme-mn-cse/ws
```

For a CSE without a location, omit `/<location>`. The Ingress forwards the
WebSocket upgrade to port 8180 and strips the external path prefix. It inherits
the HTTP Ingress's authentication, TLS, and SSL redirect settings, with read
and send timeouts of 3600 seconds.

`MQTT_ENABLE` controls MQTT and MQTT over WebSocket independently; it does not
need to be enabled for this endpoint.

To apply source changes, rebuild and publish the `meep-acme-mn-cse` image and
the `meep-virt-engine` image (which contains the chart templates), update the
running virtualisation engine, then redeploy the scenario with the updated
environment. Changing only the environment on an older MN-CSE image will not
enable WebSocket because its configuration template hardcodes `enable=false`.
