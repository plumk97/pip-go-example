package main

import (
	"errors"
	"io"
	"log"
	"net"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"runtime"
	"strconv"
	"syscall"

	"github.com/labulakalia/water"
	pipgo "github.com/plumk97/pip-go"
)

var tunIface *water.Interface

func createInterface() {
	var err error

	installWintunDLL()

	// 建立tun网卡
	tunIface, err = water.New(water.Config{
		DeviceType: water.TUN,
	})
	if err != nil {
		log.Fatalln(err)
	}

	// 设置tun IP
	args := []string{
		"interface",
		"ip",
		"set",
		"address",
		tunIface.Name(),
		"static",
		"10.0.0.1",
		"255.255.255.0",
	}

	if err := exec.Command("netsh", args...).Run(); err != nil {
		log.Fatalln(err)
	}

	tunInterface, err := net.InterfaceByName(tunIface.Name())
	if err != nil {
		log.Fatalln(err)
	}

	// 设置路由
	if output, err := exec.Command("route", "add", "1.1.1.1", "mask", "255.255.255.255", "10.0.0.1", "metric", "1", "if", strconv.Itoa(tunInterface.Index)).CombinedOutput(); err != nil {
		log.Fatalln(err)
	} else {
		log.Println(string(output))
	}

	// 监听关闭
	go func() {
		c := make(chan os.Signal)
		signal.Notify(c, os.Interrupt, syscall.SIGTERM)
		<-c

		exec.Command("route", "delete", "1.1.1.1", "mask", "255.255.255.255", "10.0.0.1", "if", strconv.Itoa(tunInterface.Index)).Run()
		tunIface.Close()
		os.Exit(0)
	}()

	packet := make([]byte, tunInterface.MTU)
	for {
		n, err := tunIface.Read(packet)
		if err != nil {
			log.Fatal(err)
		}
		pipgo.Input(packet[:n])
	}
}

func installWintunDLL() {

	var runtimeDir string
	var pwdDir string

	if path, err := os.Executable(); err == nil {
		runtimeDir = filepath.Dir(path)
	} else {
		log.Fatalln(err)
	}

	if dir, err := os.Getwd(); err == nil {
		pwdDir = dir
	} else {
		log.Fatalln(err)
	}

	var srcPath string
	switch runtime.GOARCH {
	case "amd64":
		srcPath = pwdDir + "/wintun/amd64/wintun.dll"
	case "arm":
		srcPath = pwdDir + "/wintun/arm/wintun.dll"
	case "arm64":
		srcPath = pwdDir + "/wintun/arm64/wintun.dll"
	case "386":
		srcPath = pwdDir + "/wintun/x86/wintun.dll"
	default:
		log.Fatalln(errors.New("Unsupport platform"))
	}

	src, err := os.Open(srcPath)
	if err != nil {
		log.Fatalln(err)
	}
	defer src.Close()

	dst, err := os.OpenFile(runtimeDir+"/wintun.dll", os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		log.Fatalln(err)
	}
	defer dst.Close()

	io.Copy(dst, src)
}
