package conf

import (
	"flag"
	"fmt"

	"github.com/dustin/go-humanize"
)

//Capacity is a count/size in bytes, its primary purpose is to allow the flags
// package to parse human IEC values like "10 MiB"
type Capacity int64

//String returns capacity in human readable IEC units, see: flag.Value interface
func (c *Capacity) String() string {
	return humanize.IBytes(uint64(*c))
}

//Set is used by the flag package, see: flag.Value interface
func (c *Capacity) Set(str string) error {
	val, err := humanize.ParseBytes(str)
	if err != nil {
		return fmt.Errorf("Parseing %q failed: %w", str, err)
	}

	*c = Capacity(val)
	return nil
}

//Get is used by the flag package, see: flag.Getter interface
func (c *Capacity) Get() interface{} { return Capacity(*c) }

//Analog to methods like flag.DurationVar for use in parsing capacity command-line args
func flagCapacityVar(p *Capacity, name string, value Capacity, usage string) {
	flag.CommandLine.Var(newCapacityValue(value, p), name, usage)
}

func newCapacityValue(val Capacity, p *Capacity) *Capacity {
	*p = val
	return p
}
