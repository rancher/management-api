package listener

import (
	"net/http"

	"context"

	"github.com/rancher/management-api/pkg/cert"
	"github.com/rancher/norman/types/convert"
	"github.com/rancher/types/apis/management.cattle.io/v3"
	"github.com/rancher/types/config"
	"k8s.io/apimachinery/pkg/labels"
)

type HandlerGetter func() http.Handler

type Controller struct {
	listenConfig       v3.ListenConfigInterface
	listenConfigLister v3.ListenConfigLister
	server             *Server
}

func Register(ctx context.Context, context *config.ManagementContext, httpPort, httpsPort int, getter HandlerGetter) {
	c := &Controller{
		server:             NewServer(getter, httpPort, httpsPort),
		listenConfig:       context.Management.ListenConfigs(""),
		listenConfigLister: context.Management.ListenConfigs("").Controller().Lister(),
	}
	context.Management.ListenConfigs("").AddHandler("listener", c.sync)
	go func() {
		<-ctx.Done()
		c.server.Shutdown()
	}()
}

func (c *Controller) sync(key string, listener *v3.ListenConfig) error {
	if listener == nil {
		return nil
	}

	if listener.Enabled {
		return c.enable(listener)
	} else {
		c.server.Disable(listener)
		allConfigs, err := c.listenConfigLister.List("", labels.Everything())
		if err != nil {
			return err
		}

		var lastConfig *v3.ListenConfig
		for _, config := range allConfigs {
			if !config.Enabled || config.DeletionTimestamp != nil {
				continue
			}
			if lastConfig == nil || lastConfig.CreationTimestamp.Before(&config.CreationTimestamp) {
				lastConfig = config
			}
		}

		if lastConfig != nil {
			return c.enable(listener)
		}
	}

	return nil
}

func (c *Controller) enable(listener *v3.ListenConfig) error {
	current, err := c.server.Enable(listener)
	if err != nil {
		return err
	}
	if current {
		return c.updateCurrent(listener)
	}
	return nil
}

func (c *Controller) updateCurrent(listener *v3.ListenConfig) error {
	if listener.Key != "" && listener.CACerts != "" && listener.Cert != "" {
		certInfo, err := cert.Info(listener.Cert+"\n"+listener.CACerts, listener.Key)
		if err != nil {
			return err
		}

		if certInfo.SerialNumber != listener.SerialNumber {
			copy := listener.DeepCopy()
			copy.CertFingerprint = certInfo.Fingerprint
			copy.CN = certInfo.CN
			copy.Version = certInfo.Version
			copy.ExpiresAt = convert.ToString(certInfo.ExpiresAt)
			copy.Issuer = certInfo.Issuer
			copy.IssuedAt = convert.ToString(certInfo.IssuedAt)
			copy.Algorithm = certInfo.Algorithm
			copy.SerialNumber = certInfo.SerialNumber
			copy.KeySize = certInfo.KeySize
			copy.SubjectAlternativeNames = certInfo.SubjectAlternativeNames
			_, err := c.listenConfig.Update(copy)
			if err != nil {
				return err
			}
		}
	}

	return nil
}
