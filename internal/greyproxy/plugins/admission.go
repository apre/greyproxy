package plugins

import (
	"context"

	"github.com/greyhavenhq/greyproxy/internal/gostcore/admission"
	"github.com/greyhavenhq/greyproxy/internal/gostcore/logger"
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
			"admission": "greyproxy",
		}),
	}
}

func (a *Admission) Admit(ctx context.Context, addr string, opts ...admission.Option) bool {
	a.log.Debugf("admission: admit %s", addr)
	return true
}
