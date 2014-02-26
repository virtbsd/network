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
    "encoding/json"
    "github.com/coopernurse/gorp"
    "github.com/virtbsd/VirtualMachine"
    "github.com/virtbsd/util"
    "github.com/nu7hatch/gouuid"
)

type NetworkPhysical struct {
    NetworkPhysicalID int
    NetworkUUID string
    Device string
}

type DeviceAddress struct {
    DeviceAddressID int
    DeviceUUID string
    Address string
}

type DeviceOption struct {
    DeviceOptionID int
    DeviceUUID string
    OptionKey string
    OptionValue string
}

type Network struct {
    UUID string
    Name string
    DeviceID int
    Options []*DeviceOption `db:"-"`
    Addresses []*DeviceAddress `db:"-"`
    Physicals []*NetworkPhysical `db:"-"`
}

type NetworkDevice struct {
    UUID string
    DeviceID int
    Options []*DeviceOption `db:"-"`
    Addresses []*DeviceAddress `db:"-"`
    Network *Network `db:"-"`
    NetworkUUID string
    VmUUID string
}

type Route struct {
    RouteID int
    VmUUID string
    Source string
    Destination string
}

type NetworkJSON struct {
    UUID string
    DeviceID int
    Name string
    Status string
    Options []*DeviceOption
    Addresses []*DeviceAddress
    Physicals []*NetworkPhysical
}

type NetworkDeviceJSON struct {
    UUID string
    DeviceID int
    Status string
    Options []*DeviceOption
    Addresses []*DeviceAddress
    Network *Network
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

func LookupUUID(db map[string]interface{}, field map[string]interface{}) string {
    fields := []string{ "uuid", "name" }

    if uuid, ok := field["uuid"]; ok == true {
        return uuid.(string)
    }

    for i := 0; i < len(fields); i++ {
        if val, ok := field[fields[i]]; ok == true {
            var myuuid string
            var err error

            if _, ok = db["dbmap"]; ok == true {
                myuuid, err = db["dbmap"].(*gorp.DbMap).SelectStr("select UUID from network where " + fields[i] + " = ?", val)
            } else {
                myuuid, err = db["sqleecutor"].(gorp.SqlExecutor).SelectStr("select UUID from network where " + fields[i] + " = ?", val)
            }

            if err == nil {
                return myuuid
            }
        }
    }

    return ""
}

func GetNetwork(db map[string]interface{}, field map[string]interface{}) *Network {
    var obj interface{}
    var err error

    uuid := LookupUUID(db, field)
    if len(uuid) == 0 {
        return nil
    }

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

    if obj== nil {
        return nil
    }

    return obj.(*Network)
}

func (network *Network) PostGet(s gorp.SqlExecutor) error {
    _, err := s.Select(&network.Physicals, "select * from NetworkPhysical where NetworkUUID = ?", network.UUID)
    if err != nil {
        panic(err)
    }

    _, err = s.Select(&network.Options, "select * from DeviceOption where DeviceUUID = ?", network.UUID)
    if err != nil {
        panic(err)
    }

    _, err = s.Select(&network.Addresses, "select * from DeviceAddress where DeviceUUID = ?", network.UUID)
    if err != nil {
        panic(err)
    }

    return nil
}

func (device *NetworkDevice) PostGet(s gorp.SqlExecutor) error {
    device.Network = GetNetwork(map[string]interface{}{"sqlexecutor": s}, map[string]interface{}{ "uuid": device.NetworkUUID})

    if _, err := s.Select(&device.Options, "select * from DeviceOption where DeviceUUID = ?", device.UUID); err != nil {
        panic(err)
    }

    if _, err := s.Select(&device.Addresses, "select * from DeviceAddress where DeviceUUID = ?", device.UUID); err != nil {
        panic(err)
    }

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
    for _, option := range network.Options {
        cmd.Args = append(cmd.Args, option.OptionKey)
        if len(option.OptionValue) > 0 {
            cmd.Args = append(cmd.Args, option.OptionValue)
        }
    }

    if rawoutput, err := cmd.CombinedOutput(); err != nil {
        return fmt.Errorf("ERROR: ifconfig bridge%s create: %s", id, virtbsdutil.ByteToString(rawoutput))
    }

    cmd = exec.Command("/sbin/ifconfig", "bridge" + id, "up")
    if rawoutput, err := cmd.CombinedOutput(); err != nil {
        return fmt.Errorf("ERROR: ifconfig bridge%s up: %s", id, virtbsdutil.ByteToString(rawoutput))
    }

    for _, physical := range network.Physicals {
        cmd = exec.Command("/sbin/ifconfig", "bridge" + id, "addm", physical.Device)
        if rawoutput, err := cmd.CombinedOutput(); err != nil {
            return fmt.Errorf("ERROR: ifconfig bridge%s addm %s: %s", id, physical.Device, virtbsdutil.ByteToString(rawoutput))
        }
    }

    for _, address := range network.Addresses {
        proto := "inet"
        if strings.Index(address.Address, ":") >= 0 {
            proto = "inet6"
        }

        cmd = exec.Command("/sbin/ifconfig", "bridge" + id, proto, address.Address, "alias")
        if rawoutput, err := cmd.CombinedOutput(); err != nil {
            return fmt.Errorf("ERROR: ifconfig bridge%s %s %s alias: %s", id, proto, address.Address, virtbsdutil.ByteToString(rawoutput))
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

func (network *Network) Delete(db *gorp.DbMap) error {
    if len(network.UUID) > 0 {
        _, err := db.Delete(network)
        return err
    }

    return nil
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

    cmd = exec.Command("/sbin/ifconfig", "epair" + id + "a", "up")
    if rawoutput, err := cmd.CombinedOutput(); err != nil {
        return fmt.Errorf("ERROR: ifconfig epair%sa create: %s", id, virtbsdutil.ByteToString(rawoutput))
    }

    return nil
}

func (device *NetworkDevice) BringGuestOnline(vm VirtualMachine.VirtualMachine) error {
    id := strconv.Itoa(device.DeviceID)
    vmid := vm.GetUUID()
    deviceid := "epair" + id + "b"
    needs_dhcp := false

    if vm.IsOnline() == false {
        return errors.New("VM is turned off. VM must be turned on to have its networking stack brought online")
    }

    if device.IsOnline() == false {
        if err := device.BringHostOnline(); err != nil {
            return err
        }
    }

    cmd := exec.Command("/sbin/ifconfig", deviceid, "vnet", vmid)
    for _, option := range device.Options {
        if option.OptionKey == "DHCP" {
            needs_dhcp = true
            continue
        }

        cmd.Args = append(cmd.Args, option.OptionKey)
        if len(option.OptionValue) > 0 {
            cmd.Args = append(cmd.Args, option.OptionValue)
        }
    }

    err := cmd.Run()

    if err != nil {
        return err
    }

    if needs_dhcp {
        cmd = exec.Command("/usr/sbin/jexec", vmid, "/sbin/dhclient", deviceid)
        if rawoutput, err := cmd.CombinedOutput(); err != nil {
            return fmt.Errorf("/sbin/dhclient %s: %s", deviceid, virtbsdutil.ByteToString(rawoutput))
        }
    }

    for _, address := range device.Addresses {
        proto := "inet"
        if strings.Index(address.Address, ":") >= 0 {
            proto = "inet6"
        }

        cmd = exec.Command("/usr/sbin/jexec", vmid, "/sbin/ifconfig", deviceid, proto, address.Address, "alias")
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

func (network *Network) Persist(db *gorp.DbMap) error {
    if len(network.UUID) == 0 {
        uuid, _ := uuid.NewV4()
        network.UUID = uuid.String()
        db.Insert(network)
    } else {
        db.Update(network)
    }

    for _, address := range network.Addresses {
        if len(address.DeviceUUID) == 0 {
            address.DeviceUUID = network.UUID
        }

        if address.DeviceAddressID == 0 {
            db.Insert(address)
        } else {
            db.Update(address)
        }
    }

    for _, option := range network.Options {
        option.DeviceUUID = network.UUID

        if option.DeviceOptionID == 0 {
            db.Insert(option)
        } else {
            db.Update(option)
        }
    }

    for _, physical := range network.Physicals {
        if len(physical.NetworkUUID) == 0 {
            physical.NetworkUUID = network.UUID
        }

        if physical.NetworkPhysicalID == 0 {
            db.Insert(physical);
        } else {
            db.Update(physical);
        }
    }

    return nil
}

func (device *NetworkDevice) Persist(db *gorp.DbMap, vm VirtualMachine.VirtualMachine) error {
    insert := false
    device.Network.Persist(db)

    device.NetworkUUID = device.Network.UUID

    if len(device.UUID) == 0 {
        insert = true
        uuid, _ := uuid.NewV4()
        device.UUID = uuid.String()
    }

    if insert {
        db.Insert(device)
    } else {
        db.Update(device)
    }

    for _, address := range device.Addresses {
        if len(address.DeviceUUID) == 0 {
            address.DeviceUUID = device.UUID
        }

        if address.DeviceAddressID == 0 {
            db.Insert(address)
        } else {
            db.Update(address)
        }
    }

    for _, option := range device.Options {
        option.DeviceUUID = device.UUID

        if option.DeviceOptionID == 0 {
            db.Insert(option)
        } else {
            db.Update(option)
        }
    }

    return nil
}

func (device *NetworkDevice) Delete(db *gorp.DbMap) error {
    device.Network = nil

    if len(device.UUID) > 0 {
        _, err := db.Delete(device)
        return err
    }

    return nil
}

func (network *Network) MarshalJSON() ([]byte, error) {
    obj := NetworkJSON{}
    obj.UUID = network.UUID
    obj.DeviceID = network.DeviceID
    obj.Name = network.Name
    obj.Options = network.Options
    obj.Addresses = network.Addresses
    obj.Physicals = network.Physicals

    if network.IsOnline() {
        obj.Status = "Online"
    } else {
        obj.Status = "Offline"
    }

    return json.MarshalIndent(obj, "", "    ")
}

func (device *NetworkDevice) MarshalJSON() ([]byte, error) {
    obj := NetworkDeviceJSON{}
    obj.UUID = device.UUID
    obj.DeviceID = device.DeviceID
    obj.Options = device.Options
    obj.Addresses = device.Addresses
    obj.Network = device.Network

    if device.IsOnline() {
        obj.Status = "Online"
    } else {
        obj.Status = "Offline"
    }

    bytes, err := json.MarshalIndent(obj, "", "    ")
    return bytes, err
}
