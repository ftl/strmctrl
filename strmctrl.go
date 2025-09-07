package strmctrl

import (
	"bytes"
	"context"
	"fmt"
	"image"
	"image/jpeg"
	"log"
	"time"

	"github.com/google/gousb"
)

const (
	// ImageSize is the width and height of the quadratic images for the display buttons in pixels.
	ImageSize = 64
)

const (
	vid = gousb.ID(0x1500)
	pid = gousb.ID(0x3001)

	commandTimeout = 100 * time.Millisecond
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
	Released Action = iota
	Pressed
	TurnedCW
	TurnedCCW
)

func (a Action) IsPress() bool {
	return a >= Released && a <= Pressed
}

func (a Action) IsRotation() bool {
	return a >= TurnedCW && a <= TurnedCCW
}

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

	closed chan struct{}

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
		closed: make(chan struct{}),
	}

	err = result.setupEndpoints()
	if err != nil {
		result.Close()
		return nil, fmt.Errorf("cannot setup endpoints: %w", err)
	}

	err = result.init()
	if err != nil {
		result.Close()
		return nil, fmt.Errorf("cannot initialize device: %w", err)
	}

	go result.keepAlive()

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

func (d *Device) init() error {
	err := d.sendCRTCommandWithTimeout("DIS")
	if err != nil {
		return err
	}
	return d.sendCRTCommandWithTimeout("CONNECT")
}

func (d *Device) keepAlive() {
	tick := time.NewTicker(5 * time.Second)
	defer tick.Stop()

	for {
		select {
		case <-d.closed:
			return
		case <-tick.C:
			d.sendCRTCommandWithTimeout("CONNECT")
		}
	}
}

// Close the device and clean up the used system resources.
func (d *Device) Close() {
	select {
	case <-d.closed:
		return
	default:
		close(d.closed)
	}

	d.sendCRTCommandWithTimeout("CLE", 0x00, 0xff)
	d.sendCRTCommandWithTimeout("STP")

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

func (d *Device) Descriptor() string {
	serial, _ := d.device.SerialNumber()
	return fmt.Sprintf("Bus %03d Device %03d Serial: %s", d.device.Desc.Bus, d.device.Desc.Address, serial)
}

// ReadEvents returns a channel that provides the incoming events.
// This function starts a goroutine and must only be called once.
func (d *Device) ReadEvents(ctx context.Context) (<-chan Event, error) {
	events := make(chan Event)

	go func() {
		defer close(events)

		buf := make([]byte, d.epIn.Desc.MaxPacketSize)
		tick := time.NewTicker(d.epIn.Desc.PollInterval)
		defer tick.Stop()
		for {
			select {
			case <-d.closed:
				return
			case <-tick.C:
				n, err := d.epIn.ReadContext(ctx, buf)
				if err != nil {
					continue
				}

				if n < 11 {
					log.Printf("received insufficient data from IN2 endpoint: %d", n)
				}
				event, err := newEvent(hwControl(buf[9]), buf[10])
				if err == nil { // ignore faulty events
					events <- event
				}
			}
		}
	}()

	return events, nil
}

func newEvent(control hwControl, state uint8) (Event, error) {
	switch {
	case control >= displayTopLeft && control <= displayBottomRight:
		return newPressEvent(Control(control), state)
	case control == buttonLeft:
		return newPressEvent(ButtonLeft, state)
	case control == buttonCenter:
		return newPressEvent(ButtonCenter, state)
	case control == buttonRight:
		return newPressEvent(ButtonRight, state)
	case control == knobTop:
		return newPressEvent(KnobTop, state)
	case control == knobBottomLeft:
		return newPressEvent(KnobBottomLeft, state)
	case control == knobBottomRight:
		return newPressEvent(KnobBottomRight, state)
	case control == knobTopCW, control == knobTopCCW:
		return newRotateEvent(KnobTop, control)
	case control == knobBottomLeftCW, control == knobBottomLeftCCW:
		return newRotateEvent(KnobBottomLeft, control)
	case control == knobBottomRightCW, control == knobBottomRightCCW:
		return newRotateEvent(KnobBottomRight, control)
	default:
		return Event{}, fmt.Errorf("unknown hw control: 0x%02x state: 0x%02x", control, state)
	}
}

func newPressEvent(control Control, state uint8) (Event, error) {
	return Event{
		Control: control,
		Action:  Action(state),
	}, nil
}

func newRotateEvent(control Control, hwcontrol hwControl) (Event, error) {
	action := TurnedCCW
	if hwcontrol%2 == 1 {
		action = TurnedCW
	}

	return Event{
		Control: control,
		Action:  action,
	}, nil
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
	if !display.IsDisplay() {
		return fmt.Errorf("the given control %d is not a display", display)
	}

	err := d.sendImage(ctx, uint8(display), img)
	if err != nil {
		return err
	}
	return d.sendCRTCommand(ctx, "STP")
}

// SetImages sets the images of all six display buttons at once.
func (d *Device) SetImages(ctx context.Context, imgs [6]image.Image) error {
	err := d.sendCRTCommand(ctx, "CLE", 0x00, 0xff)
	if err != nil {
		return err
	}

	for i, img := range imgs {
		if img == nil {
			continue
		}
		err = d.sendImage(ctx, uint8(i+1), img)
		if err != nil {
			return err
		}
	}

	return d.sendCRTCommand(ctx, "STP")
}

func (d *Device) sendCRTCommandWithTimeout(cmd string, args ...byte) error {
	ctx, cancel := context.WithTimeout(context.Background(), commandTimeout)
	defer cancel()

	return d.sendCRTCommand(ctx, cmd, args...)
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

func (d *Device) sendImage(ctx context.Context, index uint8, img image.Image) error {
	if img.Bounds().Max.X != ImageSize || img.Bounds().Max.Y != ImageSize {
		return fmt.Errorf("sendImage: the image must have a size of %dx%d pixels", ImageSize, ImageSize)
	}

	jpg, err := toJPEG(img)
	if err != nil {
		return err
	}

	imageSize := uint16(len(jpg))
	args := []byte{
		byte(uint8(imageSize >> 8)),
		byte(uint8(imageSize & 0x00ff)),
		index,
	}
	err = d.sendCRTCommand(ctx, "BAT", args...)
	if err != nil {
		return err
	}

	n, err := d.epOut.WriteContext(ctx, jpg)
	if err != nil {
		return err
	}
	if n < int(imageSize) {
		return fmt.Errorf("sendImage: %d bytes written, expected %d bytes", n, imageSize)
	}

	return nil
}

func toJPEG(img image.Image) ([]byte, error) {
	buffer := bytes.NewBuffer([]byte{})
	opts := jpeg.Options{
		Quality: 100,
	}
	err := jpeg.Encode(buffer, img, &opts)
	if err != nil {
		return nil, err
	}
	return buffer.Bytes(), err
}
