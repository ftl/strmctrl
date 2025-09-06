package strmctrl

import (
	"fmt"

	"github.com/google/gousb"
)

const (
	vid = gousb.ID(0x1500)
	pid = gousb.ID(0x3001)
)

type DeviceInfo struct {
	Bus     int
	Address int
	Serial  string
}

func (i DeviceInfo) String() string {
	return fmt.Sprintf("Bus %03d Device %03d: Serial %s", i.Bus, i.Address, i.Serial)
}

// List the available Stream Controller SE devices with their serial number.
func List() ([]DeviceInfo, error) {
	usb := gousb.NewContext()
	defer usb.Close()

	// OpenDevices is used to find the devices to open.
	devices, err := usb.OpenDevices(func(desc *gousb.DeviceDesc) bool {
		return desc.Vendor == vid && desc.Product == pid
	})
	if err != nil {
		return nil, fmt.Errorf("cannot enumerate devices: %w", err)
	}

	result := make([]DeviceInfo, len(devices))
	for i, device := range devices {
		serial, err := device.SerialNumber()
		if err != nil {
			return nil, fmt.Errorf("cannot read serial number from device %d: %w", i, err)
		}
		device.Close()
		result[i] = DeviceInfo{
			Bus:     device.Desc.Bus,
			Address: device.Desc.Address,
			Serial:  serial,
		}
	}

	return result, nil
}

func Open(serial string) (*Device, error) {
	return nil, fmt.Errorf("not yet implemented")
}

type Device struct {
}
