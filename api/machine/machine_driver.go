package machine

import (
	"net/http"
	"strings"

	"github.com/rancher/norman/api/access"
	"github.com/rancher/norman/types"
	"github.com/rancher/norman/types/convert"
	"github.com/rancher/types/apis/management.cattle.io/v3"
	"github.com/rancher/types/client/management/v3"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
)

type Handlers struct {
	MachineDriverClient v3.MachineDriverInterface
	MachineClient       v3.MachineInterface
	Clusters            v3.ClusterInterface
}

func (h *Handlers) ActionHandler(actionName string, action *types.Action, apiContext *types.APIContext) error {
	m, err := h.MachineDriverClient.GetNamespaced("", apiContext.ID, metav1.GetOptions{})
	if err != nil {
		return err
	}

	switch actionName {
	case "activate":
		m.Spec.Active = true
		v3.MachineDriverConditionActive.Unknown(m)
	case "deactivate":
		m.Spec.Active = false
		v3.MachineDriverConditionInactive.Unknown(m)
	}

	_, err = h.MachineDriverClient.Update(m)
	if err != nil {
		return err
	}

	data := map[string]interface{}{}
	if err := access.ByID(apiContext, apiContext.Version, apiContext.Type, apiContext.ID, &data); err != nil {
		return err
	}

	apiContext.WriteResponse(http.StatusOK, data)
	return nil
}

// FormatterDriver for MachineDriver
func (h *Handlers) FormatterDriver(apiContext *types.APIContext, resource *types.RawResource) {
	resource.AddAction(apiContext, "activate")
	resource.AddAction(apiContext, "deactivate")
}

// Formatter for Machine
func (h *Handlers) Formatter(apiContext *types.APIContext, resource *types.RawResource) {
	roles := convert.ToStringSlice(resource.Values[client.MachineFieldRole])
	if len(roles) == 0 {
		resource.Values[client.MachineFieldRole] = []string{"worker"}
	}

	machineTemplateID, ok := resource.Values["machineTemplateId"]

	// check single etcd/control node
	checkSingleNode, _ := checkSingleNode(resource, h.Clusters, h.MachineClient)

	// remove link
	if !ok || machineTemplateID == nil || checkSingleNode {
		delete(resource.Links, "remove")
	}
}

func checkSingleNode(resource *types.RawResource, clusters v3.ClusterInterface, machineInterface v3.MachineInterface) (bool, error) {
	var (
		etcd, controlplane bool
	)
	clusterID := resource.Values[client.MachineFieldClusterId].(string)
	if len(clusterID) == 0 {
		return false, nil
	}

	clusterObj, err := clusters.Get(clusterID, metav1.GetOptions{})
	if err != nil {
		return false, err
	}
	if clusterObj.Spec.RancherKubernetesEngineConfig == nil {
		return false, nil
	}

	if resource.Values[client.MachineFieldRole] == nil {
		return false, nil
	}
	machineRoles := convert.ToStringSlice(resource.Values[client.MachineFieldRole])

	if len(machineRoles) == 1 && machineRoles[0] == "worker" {
		return false, nil
	}
	machines, err := machineInterface.Controller().Lister().List(clusterID, labels.Everything())
	if err != nil {
		return false, err
	}
	for _, m := range machines {
		if m.DeletionTimestamp != nil {
			continue
		}
		nodeName := resource.Values[client.MachineFieldName].(string)
		if m.Spec.DisplayName == nodeName {
			continue
		}
		joinedRoles := strings.Join(m.Spec.Role, ",")
		if strings.Contains(joinedRoles, "controlplane") {
			controlplane = true
		}
		if strings.Contains(joinedRoles, "etcd") {
			etcd = true
		}
	}
	if !etcd || !controlplane {
		return true, nil
	}

	return false, nil
}
