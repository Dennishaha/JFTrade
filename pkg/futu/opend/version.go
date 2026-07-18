package opend

import "fmt"

const (
	minimumOpenDServerVer int32 = 1009
	minimumOpenDBuildNo   int32 = 6908
	MinimumOpenDVersion         = "10.9.6908"
)

// FormatVersion converts OpenD's encoded server version and build number into
// the version shown by Futu's downloads and release notes.
func FormatVersion(serverVer, buildNo int32) string {
	major := serverVer / 100
	minor := serverVer % 100
	if buildNo <= 0 {
		return fmt.Sprintf("%d.%d", major, minor)
	}
	return fmt.Sprintf("%d.%d.%d", major, minor, buildNo)
}

// ValidateMinimumVersion rejects OpenD releases older than the protobuf
// contract used by this repository. A nil build number is used during
// InitConnect, which only exposes serverVer; the health probe follows with the
// exact build-number check from GetGlobalState.
func ValidateMinimumVersion(serverVer int32, buildNo *int32) error {
	tooOld := serverVer < minimumOpenDServerVer
	if serverVer == minimumOpenDServerVer && buildNo != nil {
		tooOld = *buildNo < minimumOpenDBuildNo
	}
	if !tooOld {
		return nil
	}
	version := FormatVersion(serverVer, 0)
	if buildNo != nil {
		version = FormatVersion(serverVer, *buildNo)
	}
	return fmt.Errorf("OpenD 版本 %s 低于最低支持版本 %s，请升级 OpenD 后重试", version, MinimumOpenDVersion)
}
