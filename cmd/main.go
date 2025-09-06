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

var testImages = [6]image.Image{
	generateTestImage(color.RGBA{255, 0, 0, 255}),
	generateTestImage(color.RGBA{0, 255, 0, 255}),
	generateTestImage(color.RGBA{0, 0, 255, 255}),
	generateTestImage(color.RGBA{255, 255, 0, 255}),
	generateTestImage(color.RGBA{255, 0, 255, 255}),
	generateTestImage(color.RGBA{0, 255, 255, 255}),
}

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

	device.SetBrightness(ctx, 50)
	device.SetImages(ctx, testImages)

	events, err := device.ReadEvents()
	if err != nil {
		log.Fatal(err)
	}

	log.Print("device ready")

	for {
		select {
		case <-ctx.Done():
			return
		case e := <-events:
			log.Printf("%+v", e)
		}
	}
}

func generateTestImage(clr color.RGBA) image.Image {
	img := image.NewRGBA(image.Rect(0, 0, strmctrl.ImageSize, strmctrl.ImageSize))
	draw.Draw(img, img.Bounds(), image.NewUniform(clr), image.Point{}, draw.Src)
	return img
}
