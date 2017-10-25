package main

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
)

type configtype struct {              // `json:"Config"`
	Regs map[string]byte     `json:"Regs"`
	
	// Configuration
	EchoInCmdMode bool       `json:"EchoInCmdMode"`
	SpeakerMode int          `json:"SpeakerMode"`
	SpeakerVolume int        `json:"SpeakerVolume"`
	Verbose bool             `json:"Verbose"`
	Quiet bool               `json:"Quiet"`
	ConnectMsgSpeed bool     `json:"ConnectMsgSpeed"`
	BusyDetect bool          `json:"BusyDetect"`
	ExtendedResultCodes bool `json:"ExtendedResultCodes"`
	DCDControl bool          `json:"DCDControl"`
}

type storedProfiles struct {
	PowerUpConfig int                `json:"PowerUpConfig"`
	Config [2]configtype
}

func (c *configtype) Reset() {
	r := NewRegisters()
	r.Reset()
	c.Regs = r.JsonMap()
	c.EchoInCmdMode = true
	c.SpeakerMode = 1
	c.SpeakerVolume = 1
	c.Verbose = true
	c.Quiet = false
	c.ConnectMsgSpeed = true
	c.BusyDetect = true
	c.ExtendedResultCodes = true
	c.DCDControl = false
}

func newStoredProfiles() (*storedProfiles, error) {
	var c storedProfiles

	b, err := ioutil.ReadFile("hayes.config.json")
	if err != nil {
		c.PowerUpConfig = -1
		c.Config[0].Reset()
		c.Config[1].Reset()
		e := fmt.Errorf("Can't read config file: %s", err)
		logger.Print(e)
		return &c, e
	}

	if err = json.Unmarshal(b, &c); err != nil {
		logger.Print(err)
		return &c, err
	}

	return &c, nil
}

func (s *storedProfiles) Write() error {
	b, err := json.MarshalIndent(s, "", "\t")
	if err != nil {
		logger.Print(err)
		return err
	}
	err = ioutil.WriteFile("hayes.config.json", b, 0644)
	if err != nil {
		logger.Print(err)
	}
	return err
}


func (s *storedProfiles) String() string {
	b := func(p bool) (string) {
		if p {
			return "1 "
		} 
		return "0 "
	};
	i := func(p int) (string) {
		return fmt.Sprintf("%d ", p)
	};
	x := func(r, b bool) (string) {
		if (r == false && b == false) {
			return "0 "
		}
		if (r == true && b == false) {
			return "1 "
		}
		if (r == true && b == true) {
			return "7 "
		}
		return "0 "
	};
	r := func(r map[string]byte) (string) {
		reg := registersJsonUnmap(r)
		return reg.String()
	}

	var str string
	for p := 0; p < 2; p++ {
		str += fmt.Sprintf("STORED PROFILE %d\n", p)
		str += "E" + b(s.Config[p].EchoInCmdMode)
		str += "F1 "		// For Hayes 1200 compatability 
		str += "L" + i(s.Config[p].SpeakerVolume)
		str += "M" + i(s.Config[p].SpeakerMode)
		str += "Q" + b(s.Config[p].Quiet)
		str += "V" + b(s.Config[p].Verbose)
		str += "W" + b(s.Config[p].ConnectMsgSpeed)
		str += "X" +
			x(s.Config[p].ExtendedResultCodes, s.Config[p].BusyDetect)
		str += "&C" + b(s.Config[p].DCDControl)
		str += "\n"
		str += r(s.Config[p].Regs)
		str += "\n"
		if p == 0 {
			str += "\n"
		}
	}
	return str
}

func (s storedProfiles) Switch(i int) (Config, Registers, error) {
	if i != 1 &&  i != 0 {
		return Config{}, Registers{},
		fmt.Errorf("Invalid stored profile %d", i)
	}

	logger.Printf("Switching to profile %d", i)
	var c Config
	c.Reset()
	c.echoInCmdMode = s.Config[i].EchoInCmdMode 
	c.speakerVolume = s.Config[i].SpeakerVolume 
	c.speakerMode = s.Config[i].SpeakerMode 
	c.quiet = s.Config[i].Quiet 
	c.verbose = s.Config[i].Verbose 
	c.connectMsgSpeed = s.Config[i].ConnectMsgSpeed 
	c.extendedResultCodes = s.Config[i].ExtendedResultCodes 
	c.busyDetect = s.Config[i].BusyDetect 
	c.dcdControl = s.Config[i].DCDControl 
		
	return c, registersJsonUnmap(s.Config[i].Regs), nil
}

// AT&Wn
// todo pass in pointer to conf
func (s *storedProfiles) writeActive(i int) error {
	if i != 0 && i != 1 {
		return fmt.Errorf("Invalid config number %d", i)
	}

	s.Config[i].Regs = registers.JsonMap()
	s.Config[i].EchoInCmdMode = conf.echoInCmdMode
	s.Config[i].SpeakerVolume = conf.speakerVolume
	s.Config[i].SpeakerMode = conf.speakerMode
	s.Config[i].Quiet = conf.quiet
	s.Config[i].Verbose = conf.verbose
	s.Config[i].ConnectMsgSpeed = conf.connectMsgSpeed
	s.Config[i].ExtendedResultCodes = conf.extendedResultCodes
	s.Config[i].BusyDetect = conf.busyDetect
	s.Config[i].DCDControl = conf.dcdControl

	return s.Write()
}

// AT&Y
func (s *storedProfiles) setPowerUpConfig(i int) error {
	if i != 0 && i != 1 {
		return fmt.Errorf("Invalid config number %d", i)
	}
	s.PowerUpConfig = i
	return s.Write()
}
