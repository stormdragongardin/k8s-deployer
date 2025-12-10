apiVersion: kubeadm.k8s.io/v1beta4
kind: ClusterConfiguration
kubernetesVersion: {{.Version}}
imageRepository: {{.ImageRepository}}
controlPlaneEndpoint: "{{.ControlPlaneEndpoint}}"
clusterName: {{.ClusterName}}
certificatesDir: /etc/kubernetes/pki
caCertificateValidityPeriod: 876000h0m0s
certificateValidityPeriod: 876000h0m0s
encryptionAlgorithm: RSA-2048
networking:
  dnsDomain: cluster.local
  podSubnet: {{.PodSubnet}}
  serviceSubnet: {{.ServiceSubnet}}
apiServer:
  certSANs:{{if .VIP}}
  - "{{.VIP}}"{{end}}
  {{range .MasterIPs}}- "{{.}}"
  {{end}}
  extraArgs:
  - name: service-node-port-range
    value: "1-65535"
etcd:
  local:
    dataDir: /var/lib/etcd
---
apiVersion: kubeadm.k8s.io/v1beta4
kind: InitConfiguration
localAPIEndpoint:
  advertiseAddress: {{.LocalIP}}
  bindPort: 6443
nodeRegistration:
  criSocket: unix:///run/containerd/containerd.sock
  kubeletExtraArgs:
  - name: cgroup-driver
    value: systemd
---
apiVersion: kubelet.config.k8s.io/v1beta1
kind: KubeletConfiguration
cgroupDriver: systemd

