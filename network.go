/*
(BSD 2-clause license)

Copyright (c) 2014, Shawn Webb
All rights reserved.

Redistribution and use in source and binary forms, with or without modification, are permitted provided that the following conditions are met:

   * Redistributions of source code must retain the above copyright notice, this list of conditions and the following disclaimer.
   * Redistributions in binary form must reproduce the above copyright notice, this list of conditions and the following disclaimer in the documentation and/or other materials provided with the distribution.

THIS SOFTWARE IS PROVIDED BY THE COPYRIGHT HOLDERS AND CONTRIBUTORS "AS IS" AND ANY EXPRESS OR IMPLIED WARRANTIES, INCLUDING, BUT NOT LIMITED TO, THE IMPLIED WARRANTIES OF MERCHANTABILITY AND FITNESS FOR A PARTICULAR PURPOSE ARE DISCLAIMED. IN NO EVENT SHALL THE COPYRIGHT HOLDER OR CONTRIBUTORS BE LIABLE FOR ANY DIRECT, INDIRECT, INCIDENTAL, SPECIAL, EXEMPLARY, OR CONSEQUENTIAL DAMAGES (INCLUDING, BUT NOT LIMITED TO, PROCUREMENT OF SUBSTITUTE GOODS OR SERVICES; LOSS OF USE, DATA, OR PROFITS; OR BUSINESS INTERRUPTION) HOWEVER CAUSED AND ON ANY THEORY OF LIABILITY, WHETHER IN CONTRACT, STRICT LIABILITY, OR TORT (INCLUDING NEGLIGENCE OR OTHERWISE) ARISING IN ANY WAY OUT OF THE USE OF THIS SOFTWARE, EVEN IF ADVISED OF THE POSSIBILITY OF SUCH DAMAGE.
*/

package network

import (
    "github.com/coopernurse/gorp"
    "github.com/virtbsd/VirtualMachine"
    "fmt"
)

type NetworkPhysical struct {
    NetworkUUID string
    Device string
}

type DeviceAddress struct {
    DeviceUUID string
    Address string
}

type DeviceOption struct {
    DeviceUUID string
    OptionKey string
    OptionValue string
}

type Network struct {
    UUID string
    Name string
    DeviceID int
    Options map[string]string `db:"-"`
    Addresses []string `db:"-"`
    Physicals []string `db:"-"`
}

type NetworkDevice struct {
    UUID string
    DeviceID int
    Options map[string]string `db:"-"`
    Addresses []string `db:"-"`
    Network Network `db:"-"`
    NetworkUUID string
    VmUUID string
}

func GetNetworks(db *gorp.DbMap) []Network {
    var networks []Network
    _, err := db.Select(&networks, "select * from Network")

    if err != nil {
        panic(err)
        return []Network{}
    }

    return networks
}

func GetNetwork(db map[string]interface{}, uuid string) Network {
    var network Network

    if _, ok := db["dbmap"]; ok == true {
        _, err := db["dbmap"].(*gorp.DbMap).Get(&network, uuid)
        if err != nil {
            panic(err)
            return Network{}
        }
    } else {
        _, err := db["sqlexecutor"].(gorp.SqlExecutor).Get(&network, uuid)
        if err != nil {
            panic(err);
            return Network{}
        }
    }

    return network
}

func (network *Network) PostGet(s gorp.SqlExecutor) error {
    var physicals []NetworkPhysical
    var options []DeviceOption
    var addresses []DeviceAddress

    fmt.Printf("Yes, I got here with a UUID of %s\n", network.UUID)

    _, err := s.Select(&physicals, "select * from NetworkPhysical where NetworkUUID = ?", network.UUID)
    if err == nil {
        for i := 0; i < len(physicals); i++ {
            network.Physicals = append(network.Physicals, physicals[i].Device)
        }
    } else {
        panic(err)
    }

    _, err = s.Select(&options, "select * from DeviceOption where DeviceUUID = ?", network.UUID)
    if err == nil {
        for i := 0; i < len(options); i++ {
            network.Options[options[i].OptionKey] = options[i].OptionValue
        }
    } else {
        panic(err)
    }

    _, err = s.Select(&addresses, "select * from DeviceAddress where DeviceUUID = ?", network.UUID)
    if err == nil {
        for i := 0; i < len(addresses); i++ {
            network.Addresses = append(network.Addresses, addresses[i].Address)
        }
    } else {
        panic(err)
    }

    return nil
}

func (network_device *NetworkDevice) PostGet(s gorp.SqlExecutor) error {
    /* var network *Network */
    network_device.Network = GetNetwork(map[string]interface{}{"sqlexecutor": s}, network_device.NetworkUUID)

    fmt.Printf("yay!\n");

    /*
    _, err := s.Get(network, network_device.NetworkUUID)
    fmt.Printf("Looking for a network with this UUID: %s\n", network_device.NetworkUUID)
    if err == nil {
        fmt.Printf("Got it: %+v\n", network)
        network_device.Network = network
    }
    */

    return nil
}

func GetNetworkDevices(db map[string]interface{}, vm VirtualMachine.VirtualMachine) []NetworkDevice {
    var devices []NetworkDevice

    if _, ok := db["dbmap"]; ok == true {
        _, err := db["dbmap"].(*gorp.DbMap).Select(&devices, "select * from NetworkDevice where VmUUID = ?", vm.GetUUID())
        if err != nil {
            panic(err)
            return []NetworkDevice{}
        }
    } else {
        _, err := db["sqlexecutor"].(gorp.SqlExecutor).Select(&devices, "select * from NetworkDevice where VmUUID = ?", vm.GetUUID())
        if err != nil {
            panic(err);
            return []NetworkDevice{}
        }
    }

    for i := 0; i < len(devices); i++ {
        devices[i].Network = GetNetwork(db, devices[i].NetworkUUID)
        fmt.Printf("Network[%d, %s]: %+v\n", i, devices[i].NetworkUUID, devices[i].Network)
    }

    return devices
}
