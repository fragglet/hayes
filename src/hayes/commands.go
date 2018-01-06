package main

import (
	"fmt"
	"net"
	"time"
)

// Show the user what our current network status is.
func networkStatus() error {
	serial.Println("LISTENING ON:")
	ifaces, _ := net.Interfaces()
	for _, i := range ifaces {
		addrs, _ := i.Addrs()
		for _, a := range addrs {
			ip, _, _ := net.ParseCIDR(a.String())
			if !ip.IsMulticast() && !ip.IsLoopback() &&
				!ip.IsUnspecified() && !ip.IsLinkLocalUnicast() {
				serial.Printf("  Interface %s: %s\n", i.Name, ip)
			}
		}
	}
	serial.Println("ACTIVE PROTOCOLS:")
	if !flags.skipTelnet {
		serial.Printf("  Telnet (%d)\n", flags.telnetPort)
	}
	if !flags.skipSSH {
		serial.Printf("  SSH (%d)\n", flags.sshdPort)
	}

	serial.Println("ACTIVE CONNECTION:")
	if netConn != nil {
		serial.Printf("  %s\n", netConn)
	}
		
	return OK
}


// ATZn - 0 == config 0, 1 == config 1
func softReset(i int) error {
	c, r, err := profiles.Switch(i)
	if err != nil {
		return err
	}
	logger.Printf("Switching config/registers")
	factoryReset()
	conf = c
	registers = r
	time.Sleep(250 * time.Millisecond) // Cosmetic pause... 
	raiseCTS()

	return nil
}

// AT&F - reset to factory defaults
func factoryReset() error {
	err := OK
	logger.Print("Resetting modem")

	// Reset state
	goOnHook()
	setLineBusy(false)
	lowerDSR()
	lowerCTS()
	lowerRI()
	stopTimer()
	m.dcd = false
	m.lastCmd = ""
	m.lastDialed = ""
	m.connectSpeed = 0

	registers.Reset()
	conf.Reset()

	phonebook = NewPhonebook(flags.phoneBook, logger)
	err = phonebook.Load()
	if err != nil {
		logger.Print(err)
	}

	resetTimer()
	return err
}

// ATA
func answer() error {
	if offHook() {
		logger.Print("Can't answer, line off hook already")
		return ERROR
	}

	goOffHook()

	// Simulate Carrier Detect delay
	cd := registers.Read(REG_CARRIER_DETECT_RESPONSE_TIME)
	delay := time.Duration(cd) * 100 * time.Millisecond
	time.Sleep(delay)
	m.dcd = true
	m.mode = DATAMODE
	m.connectSpeed = 38400 // We only go fast...
	return CONNECT
}

// AT&V
func amperV() error {
	serial.Println("ACTIVE PROFILE:")
	serial.Println(conf.String())
	serial.Println(registers.String())

	serial.Println()
	serial.Println(profiles)

	serial.Println("TELEPHONE NUMBERS:")
	serial.Println(phonebook)

	return OK
}

// Given a parsed register command, execute it.
func registerCmd(cmd string) error {
	var err error
	var reg, val int

	// NOTE: The order of these stanzas is critical.

	// S? - query selected register
	if cmd[:2] == "S?" {
		serial.Printf("%d\n", registers.ReadCurrent())
		return nil
	}

	// Sn=x - write x to n
	_, err = fmt.Sscanf(cmd, "S%d=%d", &reg, &val)
	if err == nil {
		if reg > __NUM_REGS || reg < 0 {
			return fmt.Errorf("Register index over/underflow: %d", reg)
		}
		if val > 255 || val < 0 {
			return fmt.Errorf("Register value over/underflow: %d", val)
		}
		
		// Validate input and update modem state
		switch reg {
		case REG_AUTO_ANSWER:
			if val == 0 {
				led_AA_off()
			} else {
				led_AA_on()
			}
		case REG_ESC_CODE_GUARD_TIME:
			resetTimer()
		case REG_ESC_CH:
			escSequence[0] = byte(val)
			escSequence[1] = byte(val)
			escSequence[2] = byte(val)
		case REG_BLIND_DIAL_WAIT:
			if val < 2 {
				return ERROR
			}
		case REG_COMMA_DELAY:
			if val > 65 {
				return ERROR
			}
		case REG_BS_CH, REG_LF_CH, REG_CR_CH:
			if val > 127 {
				return ERROR
			}
		}

		registers.Write(reg, byte(val))
		return OK
	}

	// Sn? - query register n
	_, err = fmt.Sscanf(cmd, "S%d?", &reg)
	if err == nil {
		if reg > __NUM_REGS || reg < 0 {
			return fmt.Errorf("Register index over/underflow: %d", reg)
		}
		logger.Printf("Reading register %d", reg)
		serial.Printf("%d\n", registers.Read(reg))
		return OK
	}

	// Sn - slect register
	_, err = fmt.Sscanf(cmd, "S%d", &reg)
	if err == nil {
		if reg > __NUM_REGS || reg < 0 {
			return fmt.Errorf("Register index over/underflow: %d", reg)
		}
		registers.SetCurrent(reg)
		return OK
	}

	if err != nil {
		logger.Printf("registers(): err = %s", err)
	}
	return err
}

// AT&...
func processAmpersand(cmd string) error {
	if cmd[0] != '&' {
		return fmt.Errorf("Malformed AT& command: %s", cmd)
	}
	logger.Print(cmd)
	cmd = cmd[1:]

	switch cmd[0] {
	case 'C':
		conf.dcdPinned = cmd[1] == '0'
		return nil

	case 'D':
		switch cmd[1] {
		case '0': conf.dtr = 0
		case '1': conf.dtr = 1
		case '2': conf.dtr = 2
		case '3': conf.dtr = 3
		default: return fmt.Errorf("Malformed AT&D command: %s", cmd)
		}

	case 'F':
		switch cmd[1] {
		case '0':
			return factoryReset()
		}

	case 'S':
		conf.dsrPinned = cmd[1] == '0'
		return nil
		
	case 'V':
		switch cmd[1] {
		case '0':
			return amperV()
		default:
			return ERROR
		}

	case 'W':
		switch cmd[1] {
		case '0':
			return profiles.writeActive(0)
		case '1':
			return profiles.writeActive(1)
		}

	case 'Y':
		switch cmd[1] {
		case '0':
			return profiles.setPowerUpConfig(0)
		case '1':
			return profiles.setPowerUpConfig(1)
		}

	case 'Z':
		var s string
		var i int
		_, err := fmt.Sscanf(cmd, "Z%d=%s", &i, &s)
		if err != nil {
			logger.Print(err)
			return err
		}
		if s[0] == 'D' || s[0] == 'd' { // Extension
			return phonebook.Delete(i)
		}
		return phonebook.Add(i, s)

	// Faked out AT& commands
	case 'A','B','G','J','K','L','M','O','Q','R','T','U','X':
		return nil
	}

	return nil
}

// process each command
func processSingleCommand(cmd string) error {
	var status error

	switch cmd[0] {
	case 'A':
		status = answer()

	case 'Z':
		var c int
		switch cmd[1] {
		case '0':
			c = 0
		case '1':
			c = 1
		}
		status = softReset(c)

	case 'E':
		conf.echoInCmdMode = cmd[1] == '0'

	case 'F': // Online Echo mode, F1 assumed for backwards
		// compatability after Hayes 1200
		status = OK

	case 'H':
		switch cmd[1] {
		case '0':
			status = goOnHook()
		case '1':
			status = goOffHook()
		}

	case 'I':
		switch cmd[1] {
		case '0':
			serial.Println("14400")
		case '1':
			serial.Println("058") // From my Hayes Ultra 96
		case '2':
			prstatus(OK)
			serial.Println()
		case '3':
			serial.Println("04-0045012 240 PASS")
			serial.Println()
			serial.Println("04-00471-3143 080 PASS")
			serial.Println()
			serial.Println("04-00472-3143 190 PASS")
			serial.Println()
		case '4':
			serial.Println("a097841F284C6403F00000090")
			serial.Println()
			serial.Println("bF60437000")
			serial.Println()
			serial.Println("r1031111111010000")
			serial.Println()
			serial.Println("r3000111010000000")
			serial.Println()
		case '5':
			serial.Println("004")
			serial.Println("a 001 001 003 PASS")
		}
		status = OK

	case 'Q':
		conf.quiet = cmd[1] == '0'

	case 'V':
		conf.verbose = cmd[1] == '0'

	case 'L':
		switch cmd[1] {
		case '0':
			conf.speakerVolume = 0
		case '1':
			conf.speakerVolume = 1
		case '2':
			conf.speakerVolume = 2
		case '3':
			conf.speakerVolume = 3
		}

	case 'M':
		switch cmd[1] {
		case '0':
			conf.speakerMode = 0
		case '1':
			conf.speakerMode = 1
		case '2':
			conf.speakerMode = 2
		}

	case 'O':
		m.mode = COMMANDMODE 
		status = OK

	case 'W':
		switch cmd[1] {
		case '0':
			conf.connectMsgSpeed = false
		case '1', '2':
			conf.connectMsgSpeed = true
		default:
			status = ERROR
		}

	case 'X': // Change result codes displayed
		switch cmd[1] {
		case '0':
			conf.extendedResultCodes = false
			conf.busyDetect = false
		case '1', '2':
			conf.extendedResultCodes = true
			conf.busyDetect = false
		case '3', '4', '5', '6', '7':
			conf.extendedResultCodes = true
			conf.busyDetect = true
		}

	case 'D':
		status = dial(cmd)

	case 'S':
		status = registerCmd(cmd)

	case '&':
		status = processAmpersand(cmd)

	case '*':
		status = debug(cmd)

	case '!':
		status = networkStatus()

	case 'B', 'C', 'N', 'P', 'T', 'Y': // faked out commands
		status = OK

	default:
		status = OK
	}

	return status
}

func processCommands(commands []string) error {
	var cmd string
	var status error

	status = OK
	for _, cmd = range commands {
		logger.Printf("Processing: %s", cmd)
		status = processSingleCommand(cmd)
		if status != OK {
			return status
		}
	}
	return status
}
