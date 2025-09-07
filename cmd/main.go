package main

import (
	"context"
	"fmt"
	"image"
	"image/color"
	"image/draw"
	"log"
	"os"
	"os/signal"

	"github.com/ftl/strmctrl"
)

var (
	images = [6]image.Image{
		generateImage(color.RGBA{255, 0, 0, 255}),
		generateImage(color.RGBA{0, 255, 0, 255}),
		generateImage(color.RGBA{0, 0, 255, 255}),
		generateImage(color.RGBA{255, 255, 0, 255}),
		generateImage(color.RGBA{255, 0, 255, 255}),
		generateImage(color.RGBA{0, 255, 255, 255}),
	}
	brightness uint8 = 50
)

func main() {
	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt)
	defer cancel()

	if len(os.Args) <= 1 {
		list()
	} else {
		monitor(ctx, os.Args[1])
	}
}

func list() {
	deviceInfos, err := strmctrl.List()
	if err != nil {
		log.Fatal(err)
	}
	for _, info := range deviceInfos {
		fmt.Println(info.String())
	}
}

func monitor(ctx context.Context, serial string) {
	device, err := strmctrl.Open(serial)
	if err != nil {
		log.Fatal(err)
	}
	defer device.Close()

	device.Clear(ctx)
	device.SetBrightness(ctx, brightness)
	device.SetImages(ctx, images)

	events, err := device.ReadEvents(ctx)
	if err != nil {
		log.Fatal(err)
	}

	log.Printf("device %s ready", device.Descriptor())
	defer log.Print("bye")

	for {
		select {
		case <-ctx.Done():
			return
		case e := <-events:
			handleEvent(ctx, device, e)
		}
	}
}

func generateImage(clr color.RGBA) image.Image {
	img := image.NewRGBA(image.Rect(0, 0, strmctrl.ImageSize, strmctrl.ImageSize))
	draw.Draw(img, img.Bounds(), image.NewUniform(clr), image.Point{}, draw.Src)
	return img
}

func handleEvent(ctx context.Context, d *strmctrl.Device, e strmctrl.Event) {
	log.Printf("%+v", e)
	switch {
	case e.Control == strmctrl.KnobTop && e.Action.IsRotation():
		rotateImages(e.Action)
		d.SetImages(ctx, images)
	case e.Control == strmctrl.KnobBottomLeft && e.Action.IsRotation():
		brightness = max(0, min(brightness+uint8(rotationOffset(e.Action)), 100))
		d.SetBrightness(ctx, brightness)
	}
}

func rotationOffset(action strmctrl.Action) int {
	switch action {
	case strmctrl.TurnedCW:
		return 1
	case strmctrl.TurnedCCW:
		return -1
	default:
		return 0
	}
}

func rotateImages(action strmctrl.Action) {
	offset := rotationOffset(action)

	newImages := [6]image.Image{}
	for i := range images {
		newI := (i + offset) % len(images)
		if newI < 0 {
			newI = len(images) + newI
		}
		newImages[newI] = images[i]
	}
	images = newImages
}
