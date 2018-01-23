package settings

import (
	"github.com/rancher/settings"
	"github.com/rancher/types/apis/management.cattle.io/v3"
	"github.com/rancher/types/config"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/apis/meta/v1"
)

func Register(context *config.ManagementContext) error {
	sp := &settingsProvider{
		settings:       context.Management.Settings(""),
		settingsLister: context.Management.Settings("").Controller().Lister(),
	}

	return settings.SetProvider(sp)
}

type settingsProvider struct {
	settings       v3.SettingInterface
	settingsLister v3.SettingLister
}

func (s *settingsProvider) Get(name string) string {
	obj, err := s.settingsLister.Get("", name)
	if err != nil {
		return ""
	}
	if obj.Value == "" {
		return obj.Default
	}
	return obj.Value
}

func (s *settingsProvider) Set(name, value string) error {
	obj, err := s.settings.Get(name, v1.GetOptions{})
	if err != nil {
		return err
	}

	obj.Value = value
	_, err = s.settings.Update(obj)
	return err
}

func (s *settingsProvider) SetAll(settings map[string]settings.Setting) error {
	for _, setting := range settings {
		obj, err := s.settings.Get(setting.Name, v1.GetOptions{})
		if errors.IsNotFound(err) {
			newSetting := &v3.Setting{
				ObjectMeta: v1.ObjectMeta{
					Name: setting.Name,
				},
				Default: setting.Default,
			}
			_, err := s.settings.Create(newSetting)
			if err != nil {
				return err
			}
		} else if err != nil {
			return err
		} else if obj.Default != setting.Default {
			obj.Default = setting.Default
			_, err := s.settings.Update(obj)
			if err != nil {
				return err
			}
		}
	}

	return nil
}
