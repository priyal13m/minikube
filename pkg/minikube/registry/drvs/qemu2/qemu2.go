/*
Copyright 2018 The Kubernetes Authors All rights reserved.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package qemu2

import (
	"fmt"
	"io/ioutil"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"runtime"

	"github.com/docker/machine/libmachine/drivers"
	"k8s.io/minikube/pkg/drivers/qemu"

	"k8s.io/minikube/pkg/minikube/config"
	"k8s.io/minikube/pkg/minikube/download"
	"k8s.io/minikube/pkg/minikube/driver"
	"k8s.io/minikube/pkg/minikube/localpath"
	"k8s.io/minikube/pkg/minikube/registry"
)

const (
	docURL = "https://minikube.sigs.k8s.io/docs/reference/drivers/qemu2/"
)

func init() {
	if err := registry.Register(registry.DriverDef{
		Name:     driver.QEMU2,
		Init:     func() drivers.Driver { return qemu.NewDriver("", "") },
		Config:   configure,
		Status:   status,
		Default:  true,
		Priority: registry.Experimental,
	}); err != nil {
		panic(fmt.Sprintf("register failed: %v", err))
	}
}

func qemuSystemProgram() (string, error) {
	arch := runtime.GOARCH
	switch arch {
	case "amd64":
		return "qemu-system-x86_64", nil
	case "arm64":
		return "qemu-system-aarch64", nil
	default:
		return "", fmt.Errorf("unknown arch: %s", arch)
	}
}

func qemuFirmwarePath() (string, error) {
	arch := runtime.GOARCH
	// For macOS, find the correct brew installation path for qemu firmware
	if runtime.GOOS == "darwin" {
		var p, fw string
		switch arch {
		case "amd64":
			p = "/usr/local/Cellar/qemu"
			fw = "share/qemu/edk2-x86_64-code.fd"
		case "arm64":
			p = "/opt/homebrew/Cellar/qemu"
			fw = "share/qemu/edk2-aarch64-code.fd"
		default:
			return "", fmt.Errorf("unknown arch: %s", arch)
		}

		v, err := ioutil.ReadDir(p)
		if err != nil {
			return "", fmt.Errorf("lookup qemu: %v", err)
		}
		for _, version := range v {
			if version.IsDir() {
				return path.Join(p, version.Name(), fw), nil
			}
		}
	}

	switch arch {
	case "amd64":
		return "/usr/share/OVMF/OVMF_CODE.fd", nil
	case "arm64":
		return "/usr/share/AAVMF/AAVMF_CODE.fd", nil
	default:
		return "", fmt.Errorf("unknown arch: %s", arch)
	}
}

func configure(cc config.ClusterConfig, n config.Node) (interface{}, error) {
	name := config.MachineName(cc, n)
	qemuSystem, err := qemuSystemProgram()
	if err != nil {
		return nil, err
	}
	var qemuMachine string
	var qemuCPU string
	switch runtime.GOARCH {
	case "amd64":
		qemuMachine = "" // default
		qemuCPU = ""     // default
	case "arm64":
		qemuMachine = "virt"
		qemuCPU = "cortex-a72"
		// highmem=off needed, see https://patchwork.kernel.org/project/qemu-devel/patch/20201126215017.41156-9-agraf@csgraf.de/#23800615 for details
		if runtime.GOOS == "darwin" {
			qemuMachine = "virt,highmem=off"
		} else if _, err := os.Stat("/dev/kvm"); err == nil {
			qemuMachine = "virt,gic-version=3"
			qemuCPU = "host"
		}
	default:
		return nil, fmt.Errorf("unknown arch: %s", runtime.GOARCH)
	}
	qemuFirmware, err := qemuFirmwarePath()
	if err != nil {
		return nil, err
	}
	return qemu.Driver{
		BaseDriver: &drivers.BaseDriver{
			MachineName: name,
			StorePath:   localpath.MiniPath(),
			SSHUser:     "docker",
		},
		Boot2DockerURL: download.LocalISOResource(cc.MinikubeISO),
		DiskSize:       cc.DiskSize,
		Memory:         cc.Memory,
		CPU:            cc.CPUs,
		EnginePort:     2376,
		FirstQuery:     true,
		DiskPath:       filepath.Join(localpath.MiniPath(), "machines", name, fmt.Sprintf("%s.img", name)),
		Program:        qemuSystem,
		BIOS:           runtime.GOARCH != "arm64",
		MachineType:    qemuMachine,
		CPUType:        qemuCPU,
		Firmware:       qemuFirmware,
		VirtioDrives:   false,
		Network:        "user",
		CacheMode:      "default",
		IOMode:         "threads",
	}, nil
}

func status() registry.State {
	qemuSystem, err := qemuSystemProgram()
	if err != nil {
		return registry.State{Error: err, Doc: docURL}
	}

	_, err = exec.LookPath(qemuSystem)
	if err != nil {
		return registry.State{Error: err, Fix: "Install qemu-system", Doc: docURL}
	}

	qemuFirmware, err := qemuFirmwarePath()
	if err != nil {
		return registry.State{Error: err, Doc: docURL}
	}

	if _, err := os.Stat(qemuFirmware); err != nil && runtime.GOARCH == "arm64" {
		return registry.State{Error: err, Fix: "Install uefi firmware", Doc: docURL}
	}

	return registry.State{Installed: true, Healthy: true, Running: true}
}
