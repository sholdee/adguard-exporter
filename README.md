# Overview

<p>This exporter is primarily intended to run as a sidecar container for AdGuard Home running in Kubernetes, enabling metrics visibility across multiple replica instances.</p>

<img src="assets/img/agh-grafana-dash.png" width="700">

# Available metrics:
```
agh_dns_queries_total: Total number of DNS queries
agh_blocked_dns_queries_total: Total number of blocked DNS queries
agh_dns_query_types_total: DNS query types and respective counts
agh_dns_query_hosts_total: Top 100 DNS query hosts
agh_blocked_dns_query_hosts_total: Top 100 blocked DNS query hosts
agh_safe_search_enforced_hosts_total: Safe search enforced hosts
agh_dns_average_response_time: Average response time of all queries in ms
agh_dns_average_upstream_response_time: Average response time of upstream servers in ms
```

# How to use this container.

<p>Ensure that Adguard is configured to dump query log to file at regular interval using low `size_memory` setting.</p>

### AdGuardHome.yaml
```yaml
querylog:
  dir_path: ""
  ignored:
    - localhost
  interval: 168h
  size_memory: 5
  enabled: true
  file_enabled: true
```

<p>Add the sidecar container to your existing Adguard deployment manifest.</p>

### deployment.yaml
```yaml
apiVersion: apps/v1
kind: Deployment
metadata:
  name: &app adguard
  namespace: *app
  annotations:
    reloader.stakater.com/auto: "true"
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
        imagePullPolicy: IfNotPresent
        command: ["sh", "-c", "cp /home/AdGuardHome.yaml /config/AdGuardHome.yaml; chmod 755 /config/AdGuardHome.yaml"]
        volumeMounts:
          - mountPath: /home
            name: adguard-secret
          - mountPath: /config
            name: adguard-conf
      containers:
      - name: adguard-home
        image: adguard/adguardhome:v0.107.51
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
        image: sholdee/adguardexporter:v1.1.2
        ports:
        - containerPort: 8000
          name: metrics
          protocol: TCP
        volumeMounts:
        - name: adguard-data
          mountPath: /opt/adguardhome/work
      volumes:
      - emptyDir: {}
        name: adguard-data
      - emptyDir: {}
        name: adguard-conf
      - name: adguard-secret
        secret:
          secretName: adguard-secret
```

<p>Add metrics port to service definition</p>

### service.yaml
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

<p>Create a service monitor for Prometheus to start scraping metrics.</p>

### servicemonitor.yaml
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
