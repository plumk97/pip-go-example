package main

import (
	"log"
	"os/exec"
	"strconv"

	"github.com/labulakalia/water"
	pipgo "github.com/plumk97/pip-go"
)

const mtu = 9000

var tunIface *water.Interface

func createInterface() {
	var err error

	// 建立utun网卡
	tunIface, err = water.New(water.Config{
		DeviceType: water.TUN,
	})
	if err != nil {
		log.Fatalln(err)
	}
	defer tunIface.Close()

	// 设置网关地址
	cmd := exec.Command("ifconfig",
		tunIface.Name(),
		"192.168.33.1", "192.168.33.1",
		"netmask", "255.255.255.255",
		"mtu", strconv.Itoa(mtu),
		"up")

	err = cmd.Run()
	if err != nil {
		log.Fatalln(err)
	}

	// 设置路由
	if err := exec.Command("route", "-n", "add", "-net", "1.1.1.1/32", "-interface", tunIface.Name()).Run(); err != nil {
		log.Fatalln(err)
	}

	pipgo.MTU = uint16(mtu)
	packet := make([]byte, mtu)
	for {
		n, err := tunIface.Read(packet)
		if err != nil {
			log.Fatal(err)
		}
		pipgo.Input(packet[:n])
	}
}
