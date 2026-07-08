//go:build darwin

package main

/*
#cgo CFLAGS: -x objective-c
#cgo LDFLAGS: -framework Cocoa
#include <Cocoa/Cocoa.h>

static void jftradeSetAccessoryActivationPolicy(void) {
	dispatch_async(dispatch_get_main_queue(), ^{
		[NSApp setActivationPolicy:NSApplicationActivationPolicyAccessory];
	});
}
*/
import "C"

var desktopDockIconHider = hideDesktopDockIcon

func hideDesktopDockIcon() {
	C.jftradeSetAccessoryActivationPolicy()
}
