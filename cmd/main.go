package main

import (
	"fmt"
	"log"

	"github.com/ftl/strmctrl"
)

func main() {
	deviceInfos, err := strmctrl.List()
	if err != nil {
		log.Fatal(err)
	}
	for _, info := range deviceInfos {
		fmt.Println(info.String())
	}
}
