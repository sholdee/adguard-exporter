# AdGuard Exporter

This exporter is primarily intended to run as a sidecar container for AdGuard
Home in Kubernetes. It enables metrics visibility across multiple replica
instances by mounting AdGuard Home's work directory and reading lines from
`querylog.json`.

The exporter is written in Go and uses a distroless image running as nonroot.

[![AdGuard Home Grafana dashboard][dashboard-image]][dashboard-link]

## Images And Releases

Images are published to Docker Hub and GHCR:

```text
sholdee/adguardexporter
ghcr.io/sholdee/adguard-exporter
```

Releases are created manually from the GitHub Actions `Publish` workflow. Enter
a SemVer version such as `2.0.3` or `v2.0.3`; the workflow validates it, runs
tests, publishes multi-arch images, creates the `vX.Y.Z` git tag, and creates a
GitHub Release.

Each release publishes these image tags:

```text
vX.Y.Z
X.Y.Z
X.Y
latest
```

The `latest` tag points to the latest release, not the latest commit on
`master`.

## Available Metrics

```text
agh_dns_queries_total: Total number of DNS queries
agh_blocked_dns_queries_total: Total number of blocked DNS queries
agh_dns_query_types_total: DNS query types and respective counts
agh_dns_query_hosts_total: Top 100 DNS query hosts
agh_blocked_dns_query_hosts_total: Top 100 blocked DNS query hosts
agh_safe_search_enforced_hosts_total: Safe search enforced hosts
agh_dns_average_response_time: Average response time of all queries in ms
agh_dns_average_upstream_response_time: Average upstream response time in ms
```

## How To Use This Container

Configure AdGuard Home to dump query logs to disk at a regular interval by
using a low `size_memory` setting. This example causes queries to be logged
every five lines.

### `secret.yaml`

```yaml
apiVersion: v1
kind: Secret
metadata:
  name: adguard-secret
  namespace: adguard
type: Opaque
stringData:
  AdGuardHome.yaml: |
    # ... [earlier configuration omitted]

    querylog:
      dir_path: ""
      ignored:
        - localhost
      interval: 24h
      size_memory: 5
      enabled: true
      file_enabled: true

    # ... [remaining configuration omitted]
```

Add the `sholdee/adguardexporter` sidecar container to your existing AdGuard
deployment manifest.

### `deployment.yaml`

```yaml
apiVersion: apps/v1
kind: Deployment
metadata:
  name: &app adguard
  namespace: *app
spec:
  replicas: 3
  strategy:
    type: RollingUpdate
  selector:
    matchLabels:
      app: *app
  template:
    metadata:
      labels:
        app: *app
    spec:
      securityContext:
        runAsNonRoot: true
        runAsUser: 65532
        runAsGroup: 65532
        fsGroup: 65532
        seccompProfile:
          type: RuntimeDefault
      topologySpreadConstraints:
      - maxSkew: 1
        topologyKey: kubernetes.io/hostname
        whenUnsatisfiable: DoNotSchedule
        labelSelector:
          matchLabels:
            app: *app
      initContainers:
      - name: adguard-init
        image: busybox:1.36.1
        securityContext:
          capabilities:
            drop:
            - ALL
          readOnlyRootFilesystem: true
          runAsNonRoot: true
          runAsUser: 65532
          runAsGroup: 65532
          allowPrivilegeEscalation: false
        imagePullPolicy: IfNotPresent
        command:
          - sh
          - -c
          - |
            cp /home/AdGuardHome.yaml /config/AdGuardHome.yaml
            chmod 644 /config/AdGuardHome.yaml
        volumeMounts:
          - mountPath: /home
            name: adguard-secret
          - mountPath: /config
            name: adguard-conf
      containers:
      - name: adguard-home
        image: adguard/adguardhome:v0.107.52
        securityContext:
          capabilities:
            drop:
            - ALL
            add:
            - NET_BIND_SERVICE
          readOnlyRootFilesystem: true
          runAsNonRoot: true
          runAsUser: 65532
          runAsGroup: 65532
          allowPrivilegeEscalation: false
        imagePullPolicy: IfNotPresent
        ports:
        - containerPort: 53
          name: dns
          protocol: UDP
        - containerPort: 53
          name: dnstcp
          protocol: TCP
        - containerPort: 3000
          name: http-initial
          protocol: TCP
        - containerPort: 80
          name: http
          protocol: TCP
        volumeMounts:
        - name: adguard-data
          mountPath: /opt/adguardhome/work
        - name: adguard-conf
          mountPath: /opt/adguardhome/conf
        resources:
          requests:
            memory: 150Mi
            cpu: "15m"
          limits:
            memory: 400Mi
        livenessProbe: &probe
          exec:
            command:
            - /bin/sh
            - -c
            - nslookup localhost 127.0.0.1
        readinessProbe: *probe
      - name: adguard-exporter
        image: ghcr.io/sholdee/adguard-exporter:v2.0.2
        securityContext:
          capabilities:
            drop:
            - ALL
          readOnlyRootFilesystem: true
          runAsNonRoot: true
          runAsUser: 65532
          runAsGroup: 65532
          allowPrivilegeEscalation: false
        imagePullPolicy: IfNotPresent
        ports:
        - containerPort: 8000
          name: metrics
          protocol: TCP
        volumeMounts:
        - name: adguard-data
          mountPath: /opt/adguardhome/work
        livenessProbe:
          httpGet:
            path: /livez
            port: metrics
        readinessProbe:
          httpGet:
            path: /readyz
            port: metrics
      volumes:
      - emptyDir: {}
        name: adguard-data
      - emptyDir: {}
        name: adguard-conf
      - name: adguard-secret
        secret:
          secretName: adguard-secret
```

Add the metrics port to the Service definition.

### `service.yaml`

```yaml
apiVersion: v1
kind: Service
metadata:
  name: adguard-http
  namespace: adguard
  labels:
    app: adguard
spec:
  selector:
    app: adguard
  ports:
  - protocol: TCP
    port: 80
    targetPort: 80
    name: http
  - port: 8000
    protocol: TCP
    targetPort: 8000
    name: metrics
  type: ClusterIP
```

Create a ServiceMonitor for Prometheus to start scraping metrics.

### `servicemonitor.yaml`

```yaml
apiVersion: monitoring.coreos.com/v1
kind: ServiceMonitor
metadata:
  name: adguard-metrics
  namespace: adguard
  labels:
    app: adguard
spec:
  selector:
    matchLabels:
      app: adguard
  namespaceSelector:
    matchNames:
    - adguard
  endpoints:
  - port: metrics
    interval: 30s
    path: /metrics
```

[dashboard-image]: assets/img/agh-grafana-dash.png
[dashboard-link]: https://grafana.com/grafana/dashboards/21403
