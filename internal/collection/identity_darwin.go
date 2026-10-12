//go:build darwin

package collection

import (
	"errors"
	"fmt"
	"os"
	"syscall"
)

type objectToken struct {
	device          uint64
	inode           uint64
	birthTimeSecond int64
	birthTimeNano   int64
}

func (token objectToken) String() string {
	return fmt.Sprintf(
		"%d:%d:%d:%d",
		token.device,
		token.inode,
		token.birthTimeSecond,
		token.birthTimeNano,
	)
}

func directoryObjectToken(path string) (objectToken, error) {
	info, err := os.Stat(path)
	if err != nil {
		return objectToken{}, err
	}
	if !info.IsDir() {
		return objectToken{}, errors.New("source is not a directory")
	}
	stat, ok := info.Sys().(*syscall.Stat_t)
	if !ok {
		return objectToken{}, errors.New("Darwin directory identity is unavailable")
	}
	return objectToken{
		device:          uint64(stat.Dev),
		inode:           stat.Ino,
		birthTimeSecond: stat.Birthtimespec.Sec,
		birthTimeNano:   stat.Birthtimespec.Nsec,
	}, nil
}
