package hayes

import (
	"time"
	"strings"
	"fmt"
)

// ATH0
func (m *Modem) onHook() error {
	m.lowerCD()

	// It's OK to hang up the phone when there's no active network connection.
	// But if there is, close it.
	if m.conn != nil {
		m.conn.Close()
		m.conn = nil
	}

	m.onhook = true
	m.mode = COMMANDMODE
	m.connect_speed = 0
	m.led_HS_off()
	m.led_OH_off()
	return OK
}

const ON_HOOK = true
const OFF_HOOK = false

// ATH1
func (m *Modem) offHook() error {
	m.onhook = OFF_HOOK
	m.led_OH_on()
	return OK
}

func (m *Modem) getHook() bool {
	return m.onhook
}

// ATA
func (m *Modem) answer() error {
	if m.getLineBusy()  {
		m.log.Print("Can't answer, line busy")
		return ERROR
	}
	if m.getHook() == OFF_HOOK {
		m.log.Print("Can't answer, line off hook already")
		return ERROR
	}
	
	m.offHook()
	time.Sleep(400 * time.Millisecond) // Simulate Carrier Detect delay
	m.raiseCD()
	m.mode = DATAMODE
	m.connect_speed = 38400	// We only go fast...
	return CONNECT
}

// ATZ
// Setup/reset modem.  Leaves RTS & CTS down.
func (m *Modem) reset() error {
	var err error = OK

	m.log.Print("Resetting modem")

	m.onHook()
	m.lowerDSR()
	m.lowerCTS()
	m.lowerRI()

	m.echoInCmdMode = true // Echo local keypresses
	m.quiet = false		// Modem offers return status
	m.verbose = true	// Text return codes
	m.volume = 1		// moderate volume
	m.speakermode = 1	// on until other modem heard
	m.lastcmd = ""
	m.lastdialed = ""
	m.connect_speed = 0
	m.setLineBusy(false)
	m.resetRegs()
	m.addressbook, err = LoadAddressBook()
	if err != nil {
		m.log.Print(err)
	}

	return err
}

// AT&...
// Only support &V for now
func (m *Modem) ampersand(cmd string) error {
	var s string
	
	if cmd != "&V" {
		return ERROR
	}

	f := func(b bool) (t string) {
		if b {
			t += "1 "
		} else {
			t += "0 "
		}
		return t
	};

	s += "E" + f(m.echoInCmdMode)
	s += "V" + f(m.verbose)
	s += "Q" + f(m.quiet)
	s += fmt.Sprintf("M%d ", m.speakermode)
	s += "\n"
	s += m.registers.String()
	m.serial.Println(s)
	return nil
}

// process each command
func (m *Modem) processCommands(commands []string) error {
	var status error
	var cmd string

	m.log.Printf("entering PC: %+v\n", commands)
	status = OK
	for _, cmd = range commands {
		m.log.Printf("Processing: %s", cmd)
		switch cmd[0] {
		case 'A':
			status = m.answer()
		case 'Z':
			status = m.reset()
			time.Sleep(250 * time.Millisecond)
			m.raiseDSR()
			m.raiseCTS()
		case 'E':
			if cmd[1] == '0' {
				m.echoInCmdMode = false
			} else {
				m.echoInCmdMode = true
			}
		case 'F':	// Online Echo mode, F1 assumed for backwards
			        // compatability after Hayes 1200
			status = OK 
		case 'H':
			if cmd[1] == '0' { 
				status = m.onHook()
			} else if cmd[1] == '1' {
				status = m.offHook()
			} else {
				status = ERROR
			}
		case 'Q':
			if cmd[1] == '0' {
				m.quiet = true
			} else {
				m.quiet = false
			}
		case 'V':
			if cmd[1] == '0' {
				m.verbose = true
			} else {
				m.verbose = false
			}
		case 'L':
			switch cmd[1] {
			case '0': m.volume = 0
			case '1': m.volume = 1
			case '2': m.volume = 2
			case '3': m.volume = 3
			}
		case 'M':
			switch cmd[1] {
			case '0': m.speakermode = 0
			case '1': m.speakermode = 1
			case '2': m.speakermode = 2
			}
		case 'O':
			m.mode = DATAMODE
			status = OK
		case 'X':	// Change result codes displayed
			status = OK
		case 'D':
			status = m.dial(cmd)
		case 'S':
			status = m.registerCmd(cmd)
		case '&':
			status = m.ampersand(cmd)
		case '*':
			status = m.debug(cmd)
		default:
			status = ERROR
		}
		if status != OK {
			break
		}
	}
	return status
}

// Helper function to parse non-complex AT commands (everthing except ATS.., ATD...)
func parse(cmd string, opts string) (string, int, error) {

	cmd = strings.ToUpper(cmd)
	if len(cmd) == 1 {
		if cmd[0] == '/' {
			// '/' is special as it's the only true one char command
			return "/", 1, nil 
		}
		return cmd + "0", 1, nil
	}

	if strings.ContainsAny(cmd[1:2], opts) {
		return cmd[:2],  2, nil
	}

	return "", 0, fmt.Errorf("Bad command: %s", cmd)
}

// +++ 
func (m *Modem) command(cmdstring string) {
	var commands []string
	var s, opts string
	var i int
	var status error
	var err error

	// Process here is to parse the entire command string into
	// discrete commands, then execute those discrete commands in
	// the order they were given to us.  This makes syntax
	// checking/failures happen before any commands are executed
	// which is, if I recall correctly, how this works in the real
	// hardware.  Note that the command codes ("DT", "X", etc.)
	// all must be upper case for the rest of the parsing system
	// to work, but the entire command string should be left as it
	// was handed to us.  This is so that we can embed passwords
	// in the extended dial command (ATDE, specifically).


	m.log.Print("command: ", cmdstring)
	
	if len(cmdstring) < 2  {
		m.log.Print("Cmd too short")
		m.prstatus(ERROR)
		return
	}

	if strings.ToUpper(cmdstring) == "AT" {
		m.prstatus(OK)
		return
	}
	
	if strings.ToUpper(cmdstring[:2]) != "AT" {
		m.log.Print("Malformed command")
		m.prstatus(ERROR)
		return
	}

	cmd := cmdstring[2:] 		// Skip the 'AT'
	c := 0

	commands = nil
	status = OK
	savecmds := true
	for  c < len(cmd) && status == OK {
		switch (cmd[c]) {
		case 'D', 'd':
			s, i, err = parseDial(cmd[c:])
			if err != nil {
				m.prstatus(ERROR)
				return
			}
			commands = append(commands, s)
			c += i
			continue
		case 'S', 's':
			s, i, err = parseRegisters(cmd[c:])
			if err != nil {
				m.prstatus(ERROR)
				return
			}
			commands = append(commands, s)
			c += i
			continue
		case '*': 	// Custom debug registers
			s, i, err = parseDebug(cmd[c:])
			if err != nil {
				m.prstatus(ERROR)
				return
			}
			commands = append(commands, s)
			c += i
			continue
		case 'A', 'a':
			opts = "0"
		case 'E', 'e', 'H', 'h', 'Q', 'q', 'V', 'v', 'Z', 'z':
			opts = "01"
		case 'L', 'l':
			opts = "0123"
		case 'M', 'm':
			opts = "012"
		case 'O', 'o':
			opts = "O"
		case 'X', 'x':
			opts = "01234"
		case '&':
			opts = "V"
		default:
			m.log.Printf("Unknown command: %s", cmd)
			m.prstatus(ERROR)
			return
		}
		s, i, err = parse(cmd[c:], opts)
		if err != nil {
			m.prstatus(ERROR)
			return
		}
		commands = append(commands, s)
		c += i
	}

	m.log.Print("Command array: %+v", commands)
	status = m.processCommands(commands)
	m.prstatus(status)

	if savecmds && status == OK {
		m.lastcmd = cmdstring
	}
}
