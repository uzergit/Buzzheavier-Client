//go:build linux

package main

import "syscall"

const ioctlGetTermios = syscall.TCGETS
