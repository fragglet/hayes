package hayes

import (
	"fmt"
	"strings"
	"strconv"
	"time"
	"net"
)

const __CONNECT_TIMEOUT = __MAX_RINGS * 6 * time.Second

func supportedProtocol(proto string) bool {
	switch strings.ToUpper(proto) {
	case "TELNET", "SSH": return true
	default: return false
	}
}

// Using the phonebook mapping, fake out dialing a standard phone number
// (ATDT5551212)
func (m *Modem) dialNumber(phone string) (connection, error) {

	host, protocol, username, password, err := m.phonebook.Lookup(phone)
	if err != nil {
		return nil, err
	}

	m.log.Printf("Dialing address book entry: %+v", host)

	if !supportedProtocol(protocol) {
		return nil, fmt.Errorf("Unsupported protocol '%s'", protocol)
	}
	
	switch strings.ToUpper(protocol) {
	case "SSH":
		return dialSSH(host, m.log, username, password)
	case "TELNET":
		return dialTelnet(host, m.log)
	}
	return nil, fmt.Errorf("Unknown protocol")
}

func (m *Modem) dialStoredNumber(idxstr string) (connection, error) {

	index, err := strconv.Atoi(idxstr)
	if err != nil {
		m.log.Print(err)
		return nil, err
	}

	phone, err := m.phonebook.LookupStoredNumber(index)
	if err != nil {
		m.log.Print("Error: ", err)
		return nil, ERROR // We want ATDS to return ERROR.
	}
	m.log.Print("-- phone number ", phone)
	return m.dialNumber(phone)
}

// Returns host|username|password
func splitATDE(cmd string) (string, string, string, error) {
	s := strings.Split(cmd, "|")
	if len(s) != 3 {
		return "", "", "", fmt.Errorf("Malformated ATDE command")
	}
	return s[0], s[1], s[2], nil
}

// ATD...
// See http://www.messagestick.net/modem/Hayes_Ch1-1.html on ATD... result codes
func (m *Modem) dial(to string) error {
	var conn connection
	var err error

	m.goOffHook()

	cmd := to[1]
	if cmd == 'L' {
		return m.dial(m.lastdialed)
	}

	// Now we know the dial command isn't Dial Last (ATDL), save
	// this number as last dialed
	m.lastdialed = to

	// Strip out dial modifiers we don't need.
	r := strings.NewReplacer(
		",", "",
		"@", "",
		"W", "",
		" ", "",
		"!", "",
		";", "")
	
	clean_to := r.Replace(to[2:])

	switch cmd {
	case 'H':		// Hostname (ATDH hostname)
		m.log.Print("Opening telnet connection to: ", clean_to)
		conn, err = dialTelnet(clean_to, m.log)
	case 'E':		// Encrypted host (ATDE hostname)
		m.log.Print("Opening SSH connection to: ", clean_to)
		host, user, pw, e := splitATDE(clean_to)
		if e != nil {
			conn = nil
			err = e
		} else {
			conn, err = dialSSH(host, m.log, user, pw)
		}
	case 'T', 'P':		// Fake number from address book (ATDT 5551212)
		m.log.Print("Dialing fake number: ", clean_to)
		conn, err = m.dialNumber(clean_to)
	case 'S':		// Stored number (ATDS3)
		conn, err = m.dialStoredNumber(clean_to[1:])
	default:
		m.log.Printf("Dial mode '%c' not supported\n", cmd)
		m.goOnHook()
		err = fmt.Errorf("Dial mode '%c' not supported", cmd)
	}

	// if we're connected, setup the connected state in the modem,
	// otherwise return a BUSY or NO_ANSWER result code.
	if err != nil {
		m.goOnHook()
		if err == ERROR {
			return ERROR
		}
		if err, ok := err.(net.Error); ok && err.Timeout() {
			return NO_ANSWER
		}
		return BUSY
	}

	// By default, conn.Mode() will return DATAMODE here.
	// Override and stay in command mode if ; present in the
	// original command string
	ret := CONNECT
	if strings.Contains(to, ";") {
		conn.SetMode(COMMANDMODE)
		ret = OK
	}
	
	// Remote answered, hand off conneciton to handleModem()
	callChannel <- conn
	return ret
}

func parseDial(cmd string) (string, int, error) {
	var s string
	var c int
	
	c = 1			// Skip the 'D'
	switch cmd[c] {
	case 'T', 't', 'P', 'p':	// Number dialing
		e := strings.LastIndexAny(cmd, "0123456789,;@!")
		if e == -1 {
			return "", 0, fmt.Errorf("Bad phone number: %s", cmd)
		}
		s = fmt.Sprintf("DT%s", cmd[2:e+1])
		return s, len(s), nil
	case 'H', 'h':
		s = fmt.Sprintf("DH%s", cmd[c+1:])
		return s, len(s), nil
	case 'E', 'e':		// Host Dialing
		s = fmt.Sprintf("DE%s", cmd[c+1:])
		return s, len(s), nil
	case 'L', 'l':		// Dial last number
		s = fmt.Sprintf("DL")
		return s, len(s), nil
	case 'S', 's': 		// Dial stored number
		s = fmt.Sprintf("DS%s", cmd[c+1:])
		return s, len(s), nil
	}

	return "", 0, fmt.Errorf("Bad/unsupported dial command: %s", cmd)
}
