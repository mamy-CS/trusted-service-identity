kind: Service
apiVersion: v1
metadata:
  name: tsi-vault
spec:
  selector:
    app: tsi-vault
  ports:
  - protocol: TCP
    port: 8200
    targetPort: 8200
  type: NodePort
---

apiVersion: apps/v1
kind: Deployment
metadata:
  labels:
    app: tsi-vault
  name: tsi-vault
spec:
  replicas: 1
  selector:
    matchLabels:
      app: tsi-vault
  template:
    metadata:
      labels:
        app: tsi-vault
      name: tsi-vault
    spec:
      containers:
        - name: tsi-vault
          image: trustedseriviceidentity/ti-vault:<%TSI_VERSION%>
          imagePullPolicy: Always
          env:
          - name: SKIP_SETCAP
            value: "true"
          - name: SKIP_CHOWN
            value: "true"
          - name: HOME
            value: "/tmp"
