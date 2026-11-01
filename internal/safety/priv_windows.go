//go:build windows

package safety

import (
	"syscall"
	"unsafe"
)

var (
	advapi32                     = syscall.NewLazyDLL("advapi32.dll")
	procAllocateAndInitializeSid = advapi32.NewProc("AllocateAndInitializeSid")
	procCheckTokenMembership     = advapi32.NewProc("CheckTokenMembership")
	procFreeSid                  = advapi32.NewProc("FreeSid")
)

type sidIdentifierAuthority struct {
	Value [6]byte
}

var securityNtAuthority = sidIdentifierAuthority{
	Value: [6]byte{0, 0, 0, 0, 0, 5},
}

const (
	securityBuiltinDomainRid = 0x00000020
	domainAliasRidAdmins     = 0x00000220
)

// IsPrivilegedUser checks whether the current process is running with elevated
// Administrator privileges on Windows.
func IsPrivilegedUser() (bool, error) {
	var adminSid uintptr
	r1, _, err := procAllocateAndInitializeSid.Call(
		uintptr(unsafe.Pointer(&securityNtAuthority)),
		2,
		uintptr(securityBuiltinDomainRid),
		uintptr(domainAliasRidAdmins),
		0, 0, 0, 0, 0, 0,
		uintptr(unsafe.Pointer(&adminSid)),
	)
	if r1 == 0 {
		return false, err
	}
	defer procFreeSid.Call(adminSid)

	var isMember uint32
	r1, _, err = procCheckTokenMembership.Call(
		0,
		adminSid,
		uintptr(unsafe.Pointer(&isMember)),
	)
	if r1 == 0 {
		return false, err
	}

	return isMember != 0, nil
}

