# strmctrl
A Go library to control your soomfon Stream Controller SE on linux.

# Dependencies
The library uses [gousb](https://github.com/google/gousb) under the hood. You must install
[libusb-1.0](https://github.com/libusb/libusb/wiki) in order to use the strmctrl library.

# Setup
You need to setup an udev rule in order to access the Stream Controller device as normal user:

```
SUBSYSTEM=="usb", ATTRS{idVendor}=="1500", ATTRS{idProduct}=="3001", MODE:="666", GROUP="plugdev"
```

Make sure, your user is member of the `plugdev` group.

# Usage
See [`cmd/main.go`](https://github.com/ftl/strmctrl/blob/master/cmd/main.go) for an example
how to use the strmctrl library.

# License
This software is published under the [MIT License](https://www.tldrlegal.com/l/mit).

Copyright [Florian Thienel](http://thecodingflow.com/)
