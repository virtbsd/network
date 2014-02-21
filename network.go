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
    "fmt"
    "strconv"
    "errors"
    "strings"
    "os/exec"
    "github.com/coopernurse/gorp"
    "github.com/virtbsd/VirtualMachine"
    "github.com/virtbsd/util"
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
    Network *Network `db:"-"`
    NetworkUUID string
    VmUUID string
}

type Route struct {
    VmUUID string
    Source string
    Destination string
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

func GetNetwork(db map[string]interface{}, uuid string) *Network {
    var obj interface{}
    var err error

    if _, ok := db["dbmap"]; ok == true {
        obj, err = db["dbmap"].(*gorp.DbMap).Get(Network{}, uuid)
        if err != nil {
            panic(err)
            return nil
        }
    } else {
        obj, err = db["sqlexecutor"].(gorp.SqlExecutor).Get(Network{}, uuid)
        if err != nil {
            panic(err);
            return nil
        }
    }

    return obj.(*Network)
}

func (network *Network) PostGet(s gorp.SqlExecutor) error {
    var physicals []NetworkPhysical
    var options []DeviceOption
    var addresses []DeviceAddress

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

func (device *NetworkDevice) PostGet(s gorp.SqlExecutor) error {
    device.Network = GetNetwork(map[string]interface{}{"sqlexecutor": s}, device.NetworkUUID)
    return nil
}

func GetNetworkDevices(db map[string]interface{}, vm VirtualMachine.VirtualMachine) []*NetworkDevice {
    var devices []*NetworkDevice

    if _, ok := db["dbmap"]; ok == true {
        _, err := db["dbmap"].(*gorp.DbMap).Select(&devices, "select * from NetworkDevice where VmUUID = ?", vm.GetUUID())
        if err != nil {
            panic(err)
            return nil
        }
    } else {
        _, err := db["sqlexecutor"].(gorp.SqlExecutor).Select(&devices, "select * from NetworkDevice where VmUUID = ?", vm.GetUUID())
        if err != nil {
            panic(err);
            return nil
        }
    }

    return devices
}

func (network *Network) IsOnline() bool {
    id := strconv.Itoa(network.DeviceID)

    cmd := exec.Command("/sbin/ifconfig", "bridge" + id)
    err := cmd.Run()

    if err != nil {
        return false
    }

    return true
}

func (network *Network) Start() error {
    id := strconv.Itoa(network.DeviceID)

    if network.IsOnline() {
        return nil
    }

    cmd := exec.Command("/sbin/ifconfig", "bridge" + id, "create")
    for k, v := range network.Options {
        cmd.Args = append(cmd.Args, k)
        if len(v) > 0 {
            cmd.Args = append(cmd.Args, v)
        }
    }

    if rawoutput, err := cmd.CombinedOutput(); err != nil {
        return fmt.Errorf("ERROR: ifconfig bridge%s create: %s", id, virtbsdutil.ByteToString(rawoutput))
    }

    cmd = exec.Command("/sbin/ifconfig", "bridge" + id, "up")
    if rawoutput, err := cmd.CombinedOutput(); err != nil {
        return fmt.Errorf("ERROR: ifconfig bridge%s up: %s", id, virtbsdutil.ByteToString(rawoutput))
    }

    for i := range network.Physicals {
        cmd = exec.Command("/sbin/ifconfig", "bridge" + id, "addm", network.Physicals[i])
        if rawoutput, err := cmd.CombinedOutput(); err != nil {
            return fmt.Errorf("ERROR: ifconfig bridge%s addm %s: %s", id, network.Physicals[i], virtbsdutil.ByteToString(rawoutput))
        }
    }

    for i := range network.Addresses {
        proto := "inet"
        if strings.Index(network.Addresses[i], ":") >= 0 {
            proto = "inet6"
        }

        cmd = exec.Command("/sbin/ifconfig", "bridge" + id, proto, network.Addresses[i], "alias")
        if rawoutput, err := cmd.CombinedOutput(); err != nil {
            return fmt.Errorf("ERROR: ifconfig bridge%s %s %s alias: %s", id, proto, network.Addresses[i], virtbsdutil.ByteToString(rawoutput))
        }
    }

    return nil
}

func (network *Network) Stop() error {
    id := strconv.Itoa(network.DeviceID)

    if network.IsOnline() == false {
        return nil
    }

    cmd := exec.Command("/sbin/ifconfig", "bridge" + id, "destroy")
    err := cmd.Run()

    return err
}

func (device *NetworkDevice) IsOnline() bool {
    id := strconv.Itoa(device.DeviceID)

    cmd := exec.Command("/sbin/ifconfig", "epair" + id + "a")
    err := cmd.Run()

    if err != nil {
        return false
    }

    return true
}

func (device *NetworkDevice) BringHostOnline() error {
    id := strconv.Itoa(device.DeviceID)

    if device.Network.IsOnline() == false {
        if err := device.Network.Start(); err != nil {
            return err
        }
    }

    if device.IsOnline() == true {
        if err := device.BringOffline(); err != nil {
            return err
        }
    }

    cmd := exec.Command("/sbin/ifconfig", "epair" + id, "create")
    if rawoutput, err := cmd.CombinedOutput(); err != nil {
        return fmt.Errorf("ERROR: ifconfig epair%sa create: %s", id, virtbsdutil.ByteToString(rawoutput))
    }

    cmd = exec.Command("/sbin/ifconfig", "bridge" + strconv.Itoa(device.Network.DeviceID), "addm", "epair" + id + "a")
    if rawoutput, err := cmd.CombinedOutput(); err != nil {
        return fmt.Errorf("ERROR: ifconfig bridge%s addm epair%sa: %s", strconv.Itoa(device.Network.DeviceID), id, virtbsdutil.ByteToString(rawoutput))
    }

    return nil
}

func (device *NetworkDevice) BringGuestOnline(vm VirtualMachine.VirtualMachine) error {
    id := strconv.Itoa(device.DeviceID)
    vmid := vm.GetUUID()
    deviceid := "epair" + id + "b"

    if vm.IsOnline() == false {
        return errors.New("VM is turned off. VM must be turned on to have its networking stack brought online")
    }

    if device.IsOnline() == false {
        if err := device.BringHostOnline(); err != nil {
            return err
        }
    }

    cmd := exec.Command("/sbin/ifconfig", deviceid, "vnet", vmid)
    for k, v := range device.Options {
        cmd.Args = append(cmd.Args, k)
        if len(v) > 0 {
            cmd.Args = append(cmd.Args, v)
        }
    }

    err := cmd.Run()

    if err != nil {
        return err
    }

    for i := range device.Addresses {
        proto := "inet"
        if strings.Index(device.Addresses[i], ":") >= 0 {
            proto = "inet6"
        }

        cmd = exec.Command("/usr/sbin/jexec", vmid, "/sbin/ifconfig", deviceid, proto, device.Addresses[i], "alias")
        err = cmd.Run()

        if err != nil {
            device.BringOffline()
            return err
        }
    }

    cmd = exec.Command("/usr/sbin/jexec", vmid, "/sbin/ifconfig", deviceid, "up")

    return nil
}

func (device *NetworkDevice) BringOffline() error {
    id := strconv.Itoa(device.DeviceID)

    if device.IsOnline() == false {
        return nil
    }

    cmd := exec.Command("/sbin/ifconfig", "epair" + id + "a", "destroy")
    err := cmd.Run()

    return err
}
