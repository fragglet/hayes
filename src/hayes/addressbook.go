package hayes

import (
	"strings"
	"strconv"
	"encoding/csv"
	"os"
	"io"
	"fmt"
)

const __ADDRESS_BOOK_FILE = "./phonebook"

type ab_host struct {
	host string
	protocol string
	stored int 		// if 0-3, useable by AT&Z
}

func sanitizeNumber(n string) string {
	check := func(r rune) rune {
		if (r >= '0' && r <= '9') {
			return r
		} else if r == '-' || r == '(' || r == ')' || r == ' ' {
			return rune(-1)
		} else {
			return '*'
		}
	};
	return strings.Map(check, n)
}	

func isNumber(n string) (string, bool) {
	m := sanitizeNumber(n)
	if strings.Contains(m, "*") {
		return "", false
	}
	return m, true
}

func (m *Modem) printAddressBook() {

	fmt.Println("Address Book:")
	for phone, h := range m.addressbook {
		fmt.Printf(" -- Entry :%d, ph: %s, host: %s, proto: %s\n",
			h.stored, phone, h.host, h.protocol)
	}
	
}

func (m *Modem) loadAddressBook() {
	// number host protocol
	m.addressbook = make(map[string] *ab_host)

	// TODO: command line flag
	f, err := os.Open(__ADDRESS_BOOK_FILE)
	if err != nil {
		m.log.Fatal("Fatal error: ", err)
	}

	r := csv.NewReader(f)
	r.Comma = ' '
	r.Comment = '#'
	r.FieldsPerRecord = 4
	r.TrimLeadingSpace = true

	count := 0
	for {
		var i int

		record, err := r.Read()
		if err == io.EOF {
			break
		}
		if err != nil {
			m.log.Fatal("Fatal Error: ", err)
		}
		phone, ok := isNumber(record[0])
		if !ok {	// is not a valid number
			continue
		}
		if i, err = strconv.Atoi(record[3]); err != nil {
			i = -1
		}
		m.addressbook[phone] = &ab_host{record[1], record[2], i}
		count++
	}
	m.log.Printf("Address book loaded, %d hosts", count)
}

func (m *Modem) storedNumber(n int) string {

	if n < 0 || n > 3 {
		return ""
	}
	for phone, h := range m.addressbook {
		if h.stored == n {
			return phone
		}
	}
	m.log.Printf("No stored number at entry %d", n)
	return ""
}

func (m *Modem) storeNumber(phone string, pos int) int {
	// This can't be done in this implemenetation.  Return ERROR always.
	return ERROR
}
