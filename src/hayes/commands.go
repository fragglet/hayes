package main

import (
	"fmt"
	"time"
)

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
		return OK
	}

	// Sn=x - write x to n
	_, err = fmt.Sscanf(cmd, "S%d=%d", &reg, &val)
	if err == nil {
		if reg > __NUM_REGS || reg < 0 {
			logger.Printf("Register index over/underflow: %d", reg)
			return ERROR
		}
		if val > 255 || val < 0 {
			logger.Printf("Register value over/underflow: %d", val)
			return ERROR
		}

		// Update modem state
		switch reg {
		case REG_AUTO_ANSWER:
			if val == 0 {
				led_AA_off()
			} else {
				led_AA_on()
			}
		case REG_ESC_CODE_GUARD_TIME:
			if val > 255 {
				return ERROR
			}
			resetTimer()
		case REG_ESC_CH:
			escSequence[0] = byte(val)
			escSequence[1] = byte(val)
			escSequence[2] = byte(val)
		case REG_BLIND_DIAL_WAIT:
			if val < 2 || val > 255 {
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
			logger.Printf("Register index over/underflow: %d", reg)
			return ERROR
		}
		logger.Printf("Reading register %d", reg)
		serial.Printf("%d\n", registers.Read(reg))
		return OK
	}

	// Sn - slect register
	_, err = fmt.Sscanf(cmd, "S%d", &reg)
	if err == nil {
		if reg > __NUM_REGS || reg < 0 {
			logger.Printf("Register index over/underflow: %d", reg)
			return ERROR
		}
		registers.SetCurrent(reg)
		return OK
	}

	if err != nil {
		logger.Printf("registers(): err = %s", err)
	}
	return ERROR
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
		conf.dcdControl = cmd[1] == '1'
		return nil

	case 'F':
		switch cmd[1] {
		case '0':
			return factoryReset()
		}

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
	case 'A', 'B', 'D', 'G', 'J', 'K', 'L', 'M', 'O', 'Q', 'R', 'S', 'T', 'U', 'X':
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
		time.Sleep(250 * time.Millisecond)
		if status == OK {
			raiseDSR()
			raiseCTS()
		}

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
			time.Sleep(500 * time.Millisecond)
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
		m.mode = DATAMODE
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
