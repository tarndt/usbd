package impls

import (
	"math"

	"github.com/tarndt/usbd/pkg/util/consterr"
)

const zeroBlockID = math.MaxUint64

var errNotPresent = consterr.ConstErr("ID is not present in database")
