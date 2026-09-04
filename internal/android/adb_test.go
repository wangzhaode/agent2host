package android

import (
	"reflect"
	"testing"
)

func TestParseDevices(t *testing.T) {
	output := `* daemon started successfully
List of devices attached
emulator-5554          device product:sdk_phone model:Pixel_9 device:tokay transport_id:1
R58M123456             unauthorized usb:1-2 transport_id:2
192.0.2.8:5555         offline transport_id:3
`
	targets := ParseDevices("mini", output)
	if len(targets) != 3 {
		t.Fatalf("target count = %d, want 3", len(targets))
	}
	if targets[0].Host != "mini" || targets[0].Serial != "emulator-5554" || targets[0].State != "device" {
		t.Fatalf("first target = %#v", targets[0])
	}
	if targets[0].Product != "sdk_phone" || targets[0].Model != "Pixel_9" || targets[0].Codename != "tokay" || targets[0].TransportID != "1" {
		t.Fatalf("first target metadata = %#v", targets[0])
	}
	if targets[1].State != "unauthorized" || targets[1].USB != "1-2" {
		t.Fatalf("second target = %#v", targets[1])
	}
}

func TestCommandQuotesArguments(t *testing.T) {
	got, err := Command("/opt/android tools/adb", "serial:5555", "push", "/tmp/it's.apk", "/data/local/tmp/app.apk")
	if err != nil {
		t.Fatal(err)
	}
	want := `'/opt/android tools/adb' -s 'serial:5555' 'push' '/tmp/it'\''s.apk' '/data/local/tmp/app.apk'`
	if got != want {
		t.Fatalf("Command() = %q, want %q", got, want)
	}
}

func TestParseDevicesReturnsStableAttributes(t *testing.T) {
	targets := ParseDevices("local", "List of devices attached\nabc device z:3 a:1\n")
	want := map[string]string{"a": "1", "z": "3"}
	if len(targets) != 1 || !reflect.DeepEqual(targets[0].Attributes, want) {
		t.Fatalf("targets = %#v", targets)
	}
}

func TestValidateSerialRejectsUnsafeValues(t *testing.T) {
	for _, serial := range []string{"", "-d", "two devices", "bad\nserial"} {
		if err := ValidateSerial(serial); err == nil {
			t.Errorf("ValidateSerial(%q) succeeded", serial)
		}
	}
}
