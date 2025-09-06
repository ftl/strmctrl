package strmctrl

import (
	"context"
	"fmt"
	"image"

	"github.com/google/gousb"
)

const (
	// ImageSize is the width and height of the quadratic images for the display buttons in pixels.
	ImageSize = 64
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

type Control uint8

const (
	DisplayTopLeft Control = iota + 1
	DisplayTopCenter
	DisplayTopRight
	DisplayBottomLeft
	DisplayBottomCenter
	DisplayBottomRight
	ButtonLeft
	ButtonCenter
	ButtonRight
	KnobTop
	KnobBottomLeft
	KnobBottomRight
)

func (c Control) IsDisplay() bool {
	return c >= DisplayTopLeft && c <= DisplayBottomRight
}

func (c Control) IsButton() bool {
	return c >= ButtonLeft && c <= ButtonRight
}

func (c Control) IsKnob() bool {
	return c >= KnobTop && c <= KnobBottomRight
}

type Action uint8

const (
	Pressed Action = iota + 1
	Released
	TurnedCW
	TurnedCCW
)

type Event struct {
	Control Control
	Action  Action
}

type hwControl uint8

const (
	displayTopLeft      hwControl = 0x01
	displayTopCenter    hwControl = 0x02
	displayTopRight     hwControl = 0x03
	displayBottomLeft   hwControl = 0x04
	displayBottomCenter hwControl = 0x05
	displayBottomRight  hwControl = 0x06

	buttonLeft   hwControl = 0x25
	buttonCenter hwControl = 0x30
	buttonRight  hwControl = 0x31

	knobTop         hwControl = 0x35
	knobBottomLeft  hwControl = 0x33
	knobBottomRight hwControl = 0x34

	knobTopCW          hwControl = 0x51
	knobTopCCW         hwControl = 0x50
	knobBottomLeftCW   hwControl = 0x91
	knobBottomLeftCCW  hwControl = 0x90
	knobBottomRightCW  hwControl = 0x61
	knobBottomRightCCW hwControl = 0x60
)

// Device represents one Stream Controller SE that is connected via USB.
type Device struct {
	usb    *gousb.Context
	device *gousb.Device

	config *gousb.Config
	intf0  *gousb.Interface
	epIn   *gousb.InEndpoint
	epOut  *gousb.OutEndpoint
}

// Open the Stream Controller SE device with the given serial number. If the serial number
// is empty, the first available device is opened.
func Open(serial string) (*Device, error) {
	usb := gousb.NewContext()

	devices, err := usb.OpenDevices(func(desc *gousb.DeviceDesc) bool {
		return desc.Vendor == vid && desc.Product == pid
	})
	if err != nil {
		for _, device := range devices {
			if device != nil {
				device.Close()
			}
		}
		usb.Close()
		return nil, fmt.Errorf("cannot find device: %w", err)
	}

	var foundDevice *gousb.Device
	for _, device := range devices {
		if foundDevice != nil {
			device.Close()
			continue
		}

		if serial == "" {
			foundDevice = device
			continue
		}

		deviceSerial, err := device.SerialNumber()
		if err != nil {
			device.Close()
			continue
		}

		if serial == deviceSerial {
			foundDevice = device
			continue
		}

		device.Close()
	}

	if foundDevice == nil {
		usb.Close()
		return nil, fmt.Errorf("cannot find device %s", serial)
	}

	err = foundDevice.SetAutoDetach(true)
	if err != nil {
		foundDevice.Close()
		usb.Close()
		return nil, fmt.Errorf("cannot set autoDetach: %w", err)
	}
	err = foundDevice.Reset()
	if err != nil {
		foundDevice.Close()
		usb.Close()
		return nil, fmt.Errorf("cannot reset device: %v", err)
	}

	result := &Device{
		usb:    usb,
		device: foundDevice,
	}

	err = result.setupEndpoints()
	if err != nil {
		result.Close()
		return nil, fmt.Errorf("cannot setup endpoints: %w", err)
	}

	return result, nil
}

func (d *Device) setupEndpoints() error {
	var err error

	d.config, err = d.device.Config(1)
	if err != nil {
		return fmt.Errorf("cannot open config: %w", err)
	}

	d.intf0, err = d.config.Interface(0, 0)
	if err != nil {
		return fmt.Errorf("cannot get interface: %w", err)
	}

	d.epIn, err = d.intf0.InEndpoint(2)
	if err != nil {
		return fmt.Errorf("cannot create IN endpoint: %w", err)
	}

	d.epOut, err = d.intf0.OutEndpoint(3)
	if err != nil {
		return fmt.Errorf("cannot create OUT endpoint: %w", err)
	}

	return nil
}

// Close the device and clean up the used system resources.
func (d *Device) Close() {
	if d.intf0 != nil {
		d.intf0.Close()
	}
	if d.config != nil {
		d.config.Close()
	}
	if d.device != nil {
		d.device.Close()
	}
	if d.usb != nil {
		d.usb.Close()
	}
}

// ReadEvents returns a channel that provides the incoming events.
// This function starts a goroutine and must only be called once.
func (d *Device) ReadEvents() (chan Event, error) {
	return nil, fmt.Errorf("not yet implemented")
}

// SetBrightness in percent (0-100).
func (d *Device) SetBrightness(ctx context.Context, percent uint8) error {
	if percent > 100 {
		percent = 100
	}
	return d.sendCRTCommand(ctx, "LIG", percent)
}

// Clear the display buttons.
func (d *Device) Clear(ctx context.Context) error {
	err := d.sendCRTCommand(ctx, "CLE", 0x00, 0xff)
	if err != nil {
		return err
	}
	return d.sendCRTCommand(ctx, "STP")
}

// SetImage sets the image of a specific display button.
func (d *Device) SetImage(ctx context.Context, display Control, img image.Image) error {
	return fmt.Errorf("not yet implemented")
}

// SetImages sets the images of all six display buttons at once.
func (d *Device) SetImages(ctx context.Context, imgs [6]image.Image) error {
	return fmt.Errorf("not yet implemented")
}

func (d *Device) sendCRTCommand(ctx context.Context, cmd string, args ...byte) error {
	const prefix = "CRT"

	cmdBytes := make([]byte, 0, len(prefix)+2+len(cmd)+2+len(args))
	cmdBytes = append(cmdBytes, []byte(prefix)...)
	cmdBytes = append(cmdBytes, 0, 0)
	cmdBytes = append(cmdBytes, []byte(cmd)...)
	cmdBytes = append(cmdBytes, 0, 0)
	cmdBytes = append(cmdBytes, args...)

	outbuf := make([]byte, d.epOut.Desc.MaxPacketSize)
	copy(outbuf, cmdBytes)

	n, err := d.epOut.WriteContext(ctx, outbuf)
	if err != nil {
		return err
	}
	if n < len(outbuf) {
		return fmt.Errorf("sendCRTCommand: %d bytes written, expected %d bytes", n, len(outbuf))
	}

	return nil
}
