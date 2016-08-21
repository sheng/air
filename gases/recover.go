package gases

import (
	"fmt"
	"runtime"

	"github.com/sheng/air"
)

// RecoverConfig defines the config for Recover gas.
type RecoverConfig struct {
	// Skipper defines a function to skip gas.
	Skipper Skipper

	// Size of the stack to be printed.
	// Optional. Default value 4KB.
	StackSize int `json:"stack_size"`

	// DisableStackAll disables formatting stack traces of all other goroutines
	// into buffer after the trace for the current goroutine.
	// Optional. Default value false.
	DisableStackAll bool `json:"disable_stack_all"`

	// DisablePrintStack disables printing stack trace.
	// Optional. Default value as false.
	DisablePrintStack bool `json:"disable_print_stack"`
}

// DefaultRecoverConfig is the default Recover gas config.
var DefaultRecoverConfig = RecoverConfig{
	Skipper:           defaultSkipper,
	StackSize:         4 << 10, // 4 KB
	DisableStackAll:   false,
	DisablePrintStack: false,
}

// fill keeps all the fields of `RecoverConfig` have value.
func (c *RecoverConfig) fill() {
	if c.Skipper == nil {
		c.Skipper = DefaultRecoverConfig.Skipper
	}
	if c.StackSize == 0 {
		c.StackSize = DefaultRecoverConfig.StackSize
	}
}

// Recover returns a gas which recovers from panics anywhere in the chain
// and handles the control to the centralized HTTPErrorHandler.
func Recover() air.GasFunc {
	return RecoverWithConfig(DefaultRecoverConfig)
}

// RecoverWithConfig returns a Recover gas from config.
// See: `Recover()`.
func RecoverWithConfig(config RecoverConfig) air.GasFunc {
	// Defaults
	config.fill()

	return func(next air.HandlerFunc) air.HandlerFunc {
		return func(c *air.Context) error {
			if config.Skipper(c) {
				return next(c)
			}

			defer func() {
				if r := recover(); r != nil {
					var err error
					switch r := r.(type) {
					case error:
						err = r
					default:
						err = fmt.Errorf("%v", r)
					}
					stack := make([]byte, config.StackSize)
					length := runtime.Stack(stack, !config.DisableStackAll)
					if !config.DisablePrintStack {
						c.Air.Logger.Printf("[%s] %s %s", "PANIC RECOVER", err, stack[:length])
					}
					c.Air.HTTPErrorHandler(err, c)
				}
			}()
			return next(c)
		}
	}
}
