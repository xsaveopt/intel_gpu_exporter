package sysutil

import (
	"os"
	"strconv"
	"strings"
)

func ReadString(path string) (string, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(b)), nil
}

func ReadUint64(path string) (uint64, error) {
	s, err := ReadString(path)
	if err != nil {
		return 0, err
	}
	return strconv.ParseUint(s, 10, 64)
}

func ReadFloat64(path string) (float64, error) {
	s, err := ReadString(path)
	if err != nil {
		return 0, err
	}
	return strconv.ParseFloat(s, 64)
}
