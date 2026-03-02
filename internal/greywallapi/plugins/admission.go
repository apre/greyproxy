package plugins

import (
	"context"

	"github.com/go-gost/core/admission"
	"github.com/go-gost/core/logger"
)

// Admission implements admission.Admission.
// It always admits all source connections. Access control is deferred to the bypass plugin.
type Admission struct {
	log logger.Logger
}

func NewAdmission() *Admission {
	return &Admission{
		log: logger.Default().WithFields(map[string]any{
			"kind":      "admission",
			"admission": "greywallapi",
		}),
	}
}

func (a *Admission) Admit(ctx context.Context, addr string, opts ...admission.Option) bool {
	a.log.Debugf("admission: admit %s", addr)
	return true
}
