package assigned_address

import (
	"context"
	"encoding/json"
	"net"

	"github.com/pkg/errors"
	log "github.com/sirupsen/logrus"

	"github.com/runityru/anycastd/checkers"
)

var _ checkers.Checker = (*assigned_address)(nil)

type assigned_address struct {
	iface *string
	ipv4  string

	interfaceCollector func() (map[string]string, error)
}

const checkName = "assigned_address"

func init() {
	checkers.Register(checkName, NewFromSpec)
}

func New(s spec) (checkers.Checker, error) {
	if err := s.Validate(); err != nil {
		return nil, err
	}

	return &assigned_address{
		iface: s.Interface,
		ipv4:  s.IPv4,

		interfaceCollector: gatherInterfaces,
	}, nil
}

func NewFromSpec(in json.RawMessage) (checkers.Checker, error) {
	s := spec{}
	if err := json.Unmarshal(in, &s); err != nil {
		return nil, err
	}

	return New(s)
}

func (h *assigned_address) Kind() string {
	return checkName
}

func (d *assigned_address) Check(ctx context.Context) error {
	ifaces, err := d.interfaceCollector()
	if err != nil {
		return errors.Wrap(err, "error discovering network interfaces")
	}

	log.WithFields(log.Fields{
		"check":      checkName,
		"interfaces": ifaces,
	}).Tracef("discovered interfaces")

	v, ok := ifaces[d.ipv4]
	if !ok {
		return errors.New("no IPv4 address found on the system")
	}

	if d.iface != nil && *d.iface != v {
		return errors.New("Interface name is not matched for described IPv4 address")
	}

	return nil
}

func gatherInterfaces() (map[string]string, error) {
	ifaces, err := net.Interfaces()
	if err != nil {
		return nil, err
	}

	interfaces := map[string]string{}
	for _, iface := range ifaces {
		addrs, err := iface.Addrs()
		if err != nil {
			return nil, err
		}

		for _, addr := range addrs {
			var ip net.IP
			switch v := addr.(type) {
			case *net.IPNet:
				ip = v.IP
			case *net.IPAddr:
				ip = v.IP
			default:
				return nil, errors.Errorf("unexpected address type: %T", v)
			}

			interfaces[ip.String()] = iface.Name
		}
	}

	return interfaces, nil
}
