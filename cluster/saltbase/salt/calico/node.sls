{% if pillar.get('policy_provider', '').lower() == 'calico' %}

calicoctl:
  file.managed:
    - name: /usr/bin/calicoctl
    - source: https://github.com/projectcalico/calico-docker/releases/download/v0.19.0/calicoctl
    - source_hash: sha256=6db00c94619e82d878d348c4e1791f8d2f0db59075f6c8e430fefae297c54d96
    - makedirs: True
    - mode: 744

calico-policy:
  file.managed:
    - name: /usr/bin/policy
    - source: https://github.com/projectcalico/k8s-policy/releases/download/v0.1.4/policy
    - source_hash: sha256=def1b53ec0bf3ec2dce9edb7b4252a514ccd6b06c7e738a324e0a3e9ecf12bbe
    - makedirs: True
    - mode: 744

calico-node:
  cmd.run:
    - name: calicoctl node
    - unless: docker ps | grep calico-node
    - env:
      - ETCD_AUTHORITY: "{{ grains.api_servers }}:6666"
      - CALICO_NETWORKING: "false"
    - require:
      - kmod: ip6_tables
      - kmod: xt_set
      - service: docker
      - file: calicoctl

calico-cni:
  file.managed:
    - name: /opt/cni/bin/calico
    - source: https://github.com/projectcalico/calico-cni/releases/download/v1.3.0/calico
    - source_hash: sha256=2f65616cfca7d7b8967a62f179508d30278bcc72cef9d122ce4a5f6689fc6577
    - makedirs: True
    - mode: 744

calico-cni-config:
  file.managed:
    - name: /etc/cni/net.d/10-calico.conf
    - source: salt://calico/10-calico.conf
    - makedirs: True
    - mode: 644
    - template: jinja

calico-update-cbr0:
  cmd.run:
    - name: sed -i "s#CBR0_CIDR#$(ip addr list docker0 | grep -oP 'inet \K\S+')#" /etc/cni/net.d/10-calico.conf
    - require:
      - file: calico-cni
      - file: calico-cni-config
      - cmd: calico-node
      - service: kubelet
      - service: docker

calico-restart-kubelet:
  cmd.run:
    - name: service kubelet restart
    - require:
      - cmd: calico-update-cbr0

ip6_tables:
  kmod.present

xt_set:
  kmod.present

{% endif -%}
