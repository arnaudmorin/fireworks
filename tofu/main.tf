terraform {
  required_version = ">= 1.6"
  required_providers {
    openstack = {
      source  = "terraform-provider-openstack/openstack"
      version = "~> 3.0"
    }
  }
}

provider "openstack" {
  region = var.region
}

resource "openstack_compute_keypair_v2" "admin" {
  name       = "${var.name}-admin"
  public_key = file(pathexpand(var.ssh_public_key_file))
}

resource "openstack_networking_secgroup_v2" "ssh" {
  name        = "${var.name}-ssh"
  description = "Public SSH for the fireworks show"
}

resource "openstack_networking_secgroup_rule_v2" "ssh" {
  for_each          = toset(["IPv4", "IPv6"])
  security_group_id = openstack_networking_secgroup_v2.ssh.id
  direction         = "ingress"
  ethertype         = each.key
  protocol          = "tcp"
  port_range_min    = 22
  port_range_max    = 22
  remote_ip_prefix  = each.key == "IPv4" ? "0.0.0.0/0" : "::/0"
}

resource "openstack_compute_instance_v2" "fireworks" {
  name            = var.name
  image_name      = var.image_name
  flavor_name     = var.flavor_name
  key_pair        = openstack_compute_keypair_v2.admin.name
  security_groups = [openstack_networking_secgroup_v2.ssh.name]
  user_data       = file("${path.module}/../userdata.sh")
  metadata        = { for k, v in { fireworks_message = var.message } : k => v if v != null }

  network {
    name = var.network_name
  }

  lifecycle {
    replace_triggered_by = [terraform_data.message]
  }
}

# userdata.sh only reads the message at boot
resource "terraform_data" "message" {
  input = var.message
}
