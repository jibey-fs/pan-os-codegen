resource "panos_high_availability" "example" {
  location = {
    template = {
      name = "example-template"
    }
  }

  enabled = true

  interface = {
    ha1        = { port = "ha1-a", ip_address = "$ha1-local-ip" }
    ha1_backup = { port = "ha1-b", ip_address = "$ha1-backup-local-ip" }
    ha2        = { port = "ethernet1/19" }
    ha2_backup = { port = "ethernet1/20" }
    ha3        = { port = "ae1" }
  }

  group = {
    group_id       = "1"
    description    = "active/active route-based HA"
    peer_ip        = "$ha-peer-ha1-ip"
    peer_ip_backup = "$ha-backup-peer-ha1-ip"

    election_option = {
      device_priority  = "$ha-device-priority"
      heartbeat_backup = true
      preemptive       = true
    }

    state_synchronization = {
      ha2_keep_alive = {
        enabled = true
        action  = "split-datapath"
      }
    }

    mode = {
      active_active = {
        device_id = "0"
        session_owner_selection = {
          first_packet = {}
        }
      }
    }
  }
}
