package usbdlib

import (
	"bufio"
	"bytes"
	"errors"
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"

	"github.com/pmorjan/kmod"
	"golang.org/x/sys/unix"
)

//This is a collection of utilities for managing the Linux kernel NBD module

func useNbdDev(devPath string) error {
	err := nbdLoaded()
	if err != nil {
		return fmt.Errorf("NBD is not loaded therefore %q could not possibly be an NBD: %w", devPath, err)
	}

	if err = validateDevPath(devPath); err != nil {
		return fmt.Errorf("%q is not a usable NBD: %w", devPath, err)
	}
	return nil
}

func makeNbdDev(maxNBDDevices uint) (string, error) {
	err := nbdLoaded()
	if err != nil {
		if err = loadNbd(maxNBDDevices); err != nil {
			return "", fmt.Errorf("NBD was not loaded and an attempt to load it failed: %w", err)
		}
	}

	devPath, err := nbdFreeDev()
	if err != nil {
		return "", fmt.Errorf("Did not find usable NBD: %w", err)
	}
	return devPath, nil
}

func nbdLoaded() error {
	const (
		procfsKMods = "/proc/modules"
		nbdModName  = "nbd"
		liveFlag    = "Live"
	)

	fin, err := os.Open(procfsKMods)
	if err != nil {
		return fmt.Errorf("Could not open procfs file %q listing kernel modules: %w", procfsKMods, err)
	}
	defer fin.Close()

	var loaded, live bool
	lines := bufio.NewScanner(fin)
	for lines.Scan() {
		if line := lines.Bytes(); bytes.HasPrefix(line, []byte(nbdModName)) {
			loaded, live = true, bytes.Contains(line, []byte(liveFlag))
			break
		}
	}
	if err = lines.Err(); err != nil {
		return fmt.Errorf("Could not read procfs file %q listing kernel modules: %w", procfsKMods, err)
	}

	switch {
	case err != nil:
		return fmt.Errorf("Could not determine if the NDB kernel module is loaded: %w", err)
	case !loaded:
		return fmt.Errorf("The NDB kernel module is not loaded")
	case !live:
		return fmt.Errorf("The NDB kernel module is loaded but is not live")
	}
	return nil
}

func loadNbd(maxNBDDevices uint) error {
	if maxNBDDevices < 1 {
		return fmt.Errorf("Loading NBD with fewer than 1 devices provisioned is pointless")
	}

	kmodLoader, err := kmod.New()
	if err != nil {
		return fmt.Errorf("Failed to contruct kernel module loader: %w", err)
	}

	const ndbModuleName = "nbd"
	ndbModeuleArg := fmt.Sprintf("nbds_max=%d", maxNBDDevices)

	if err = kmodLoader.Load(ndbModuleName, ndbModeuleArg, 0); err != nil {
		switch {
		case errors.Is(err, kmod.ErrModuleNotFound), errors.Is(err, kmod.ErrModuleInUse):
			return fmt.Errorf("Failed to load kernel module %q: %w", ndbModuleName, err)
		}
		return fmt.Errorf("Failed to load kernel module %q (ensure this binary has capability \"cap_sys_module\" or root. %s: %w", ndbModuleName, capHelp(), err)
	}

	if err = nbdLoaded(); err != nil {
		return fmt.Errorf("NBD did not appear as loaded after load attempt: %w", err)
	}
	return nil
}

func capHelp() string {
	return fmt.Sprintf("(Ex. run \"sudo setcap cap_sys_module+ep %s\")", os.Args[0])
}

func validateDevPath(devPath string) error {
	fstat, err := os.Stat(devPath)
	switch {
	case errors.Is(err, os.ErrNotExist):
		return fmt.Errorf("Provided device %q does not exist", devPath)
	case err != nil:
		return fmt.Errorf("Provided device %q could not be stat'd", devPath)
	case fstat.IsDir():
		return fmt.Errorf("Provided device %q is a directory", devPath)
	case fstat.Mode().IsRegular():
		return fmt.Errorf("Provided device %q is not special therefore cannot be a device file", devPath)
	case fstat.Size() > 0:
		return fmt.Errorf("Provided device %q has a size greater than zero so it may be in use", devPath)
	}

	devMajor, err := nbdMajor()
	if err != nil {
		return fmt.Errorf("Could not determine major device number for NDB: %w", err)
	}

	if actualMajor, _, err := ndbMajorMinorForDevice(devPath); err != nil {
		return fmt.Errorf("Could not validate device major and minor were correct for %q: %w", devPath, err)
	} else if actualMajor != devMajor {
		return fmt.Errorf("Create device %q had device major number %d when %d was expected", devPath, actualMajor, devMajor)
	}
	return nil
}

func nbdMajor() (int, error) {
	const (
		procfsDeviceMajors = "/proc/devices"
		nbdDevName         = "nbd"
		nbdDevSuffix       = " " + nbdDevName
	)

	fin, err := os.Open(procfsDeviceMajors)
	if err != nil {
		return -1, fmt.Errorf("Could not open procfs file %q listing kernel device major numbers: %w", procfsDeviceMajors, err)
	}
	defer fin.Close()

	lines := bufio.NewScanner(fin)
	for lines.Scan() {
		if line := lines.Bytes(); bytes.HasSuffix(line, []byte(nbdDevSuffix)) {
			devMajor, err := strconv.Atoi(string(bytes.TrimSpace(bytes.TrimSuffix(line, []byte(nbdDevSuffix)))))
			if err != nil {
				return -1, fmt.Errorf("Could not parse %q device major entry: %q in %q: %w", nbdDevName, line, procfsDeviceMajors, err)
			}
			return devMajor, nil
		}
	}
	if err = lines.Err(); err != nil {
		return -1, fmt.Errorf("Could not read procfs file %q listing device major numbers: %w", procfsDeviceMajors, err)
	}
	return -1, fmt.Errorf("Could not find device major entry in %q for %q", procfsDeviceMajors, nbdDevName)
}

func ndbMajorMinorForDevice(devPath string) (major, minor int, err error) {
	fstat := syscall.Stat_t{}
	if err = syscall.Stat(devPath, &fstat); err != nil {
		return -1, -1, fmt.Errorf("Could not stat provided potential device file %q: %w", devPath, err)
	}

	if fmode := os.FileMode(fstat.Mode); fmode&os.ModeDevice != 0 {
		return -1, -1, fmt.Errorf("Provided device file %q is not a block device: %w", devPath, err)
	}

	return int(unix.Major(fstat.Rdev)), int(unix.Minor(fstat.Rdev)), nil
}

func nbdFreeDev() (string, error) {
	const sysblockDevsDir = "/sys/block/"
	dirs, err := ioutil.ReadDir(sysblockDevsDir)
	if err != nil {
		return "", fmt.Errorf("Could not list blocks devices in sysfs directory %q: %w", sysblockDevsDir, err)
	}

	const (
		nbdDevPrefix = "nbd"
		devDir       = "/dev"
	)
	devCount := 0
	for _, dir := range dirs {
		devName := dir.Name()
		if !strings.HasPrefix(devName, nbdDevPrefix) {
			continue
		}
		devCount++

		devPath := filepath.Join(devDir, devName)
		if err = validateDevPath(devPath); err != nil {
			continue
		}
		return devPath, nil
	}
	return "", fmt.Errorf("None of %d NDB devices found were free (empty) and valid (correct device major number)", devCount)
}
