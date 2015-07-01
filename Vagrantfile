# -*- mode: ruby -*-
# vi: set ft=ruby :

# All our VMs will reside in this subnet
SUBNET='192.168.42'

# The VM name of the singular ceph VM
CEPH_VM_NAME = 'ceph'

# The IP address of the ceph VM
CEPH_VM_IP = 100

# All Vagrant configuration is done below. The "2" in Vagrant.configure
# configures the configuration version (we support older styles for
# backwards compatibility). Please don't change it unless you know what
# you're doing.
Vagrant.configure(2) do |config|

  # Every Vagrant development environment requires a box.
  config.vm.box = 'ubuntu/trusty64'
  config.ssh.insert_key = false

  config.vm.define CEPH_VM_NAME do |ceph|

    ceph.vm.hostname = CEPH_VM_NAME

    # Create a private network, which allows host-only access to the machine
    # using a specific IP.
    ceph.vm.network 'private_network', ip: "#{SUBNET}.#{CEPH_VM_IP}"

    # Add a 1 GB drive to the VM to act as OSD drive
    ceph.vm.provider 'virtualbox' do |vbox|

      # Display the VirtualBox GUI when booting the machine
      vbox.gui = false

      vbox.name = CEPH_VM_NAME
      vbox.cpus = 2
      vbox.memory = 1024


      1.times do |i|
        vbox.customize %W(createhd --filename .vagrant/ceph-#{i} --size 1000)
        vbox.customize ['storageattach', :id, '--storagectl', 'SATAController', '--port', 3 + i, '--device', 0, '--type', 'hdd', '--medium', ".vagrant/ceph-#{i}.vdi"]
      end

    end

    # Provision the ceph VM using ceph-ansible
    ceph.vm.provision 'ansible' do |ansible|

      ansible.playbook = 'ceph-ansible/site.yml'

      # Note: Can't do ranges like mon[0-2] in groups because
      # these aren't supported by Vagrant, see
      # https://github.com/mitchellh/vagrant/issues/3539
      ansible.groups = {
        'mons' => [CEPH_VM_NAME],
        'osds' => [CEPH_VM_NAME],
        'mdss' => [CEPH_VM_NAME],
        'rgws' => []
      }

      ansible.extra_vars = {
        ceph_stable: true, # use ceph stable branch

        restapi: false, # disable restapi configuration in ceph.conf

        cephx_require_signatures: false,

        pool_default_pg_num: 32,
        pool_default_pgp_num: 32,

        # These are the same ones from ceph-ansible/Vagrantfile
        # In a production deployment, these should be secret
        fsid: '4a158d27-f750-41d5-9e7f-26ce4c9d2d45',
        monitor_secret: 'AQAWqilTCDh7CBAAawXt6kyTgLFCxSvJhTEmuw==',
        radosgw: 'false',
        mds: 'false',

        # Needed for single node
        common_single_host_mode: 'True',

        # Change to > 1 if you have more than one OSD
        pool_default_size: 1,

        # See http://ceph.com/docs/master/rados/configuration/osd-config-ref/
        journal_size: 48,

        monitor_interface: 'eth1',

        # Has to reflect the same IP block in Vagrantfile
        cluster_network: "#{SUBNET}.0/24",
        public_network: "#{SUBNET}.0/24"
      }

      ansible.limit = 'all'
    end

    ceph.vm.provision 'ansible' do |ansible|
      ansible.playbook = 'playbook.yml'
    end

  end

  # If true, then any SSH connections made will enable agent forwarding.
  # Default value: false
  config.ssh.forward_agent = true

end
