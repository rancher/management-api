package rbac

import (
	"github.com/rancher/types/apis/rbac.authorization.k8s.io/v1"
	rbacv1 "k8s.io/api/rbac/v1"
	"k8s.io/client-go/tools/cache"
)

const (
	rbacGroup = "rbac.authorization.k8s.io"
)

func newIndexes(client v1.Interface) (user *permissionIndex, group *permissionIndex) {
	client.ClusterRoleBindings("").Controller().Informer().
		AddIndexers(cache.Indexers{
			"crbUser": func(obj interface{}) ([]string, error) {
				return clusterRoleBindingIndexer("User", obj)
			},
			"crbGroup": func(obj interface{}) ([]string, error) {
				return clusterRoleBindingIndexer("Group", obj)
			},
		})

	client.RoleBindings("").Controller().Informer().
		AddIndexers(cache.Indexers{
			"rbUser": func(obj interface{}) ([]string, error) {
				return roleBindingIndexer("User", obj)
			},
			"rbGroup": func(obj interface{}) ([]string, error) {
				return roleBindingIndexer("Group", obj)
			},
		})

	user = &permissionIndex{
		clusterRoleLister:   client.ClusterRoles("").Controller().Lister(),
		roleLister:          client.Roles("").Controller().Lister(),
		crbIndexer:          client.ClusterRoleBindings("").Controller().Informer().GetIndexer(),
		rbIndexer:           client.RoleBindings("").Controller().Informer().GetIndexer(),
		roleIndexKey:        "rbUser",
		clusterRoleIndexKey: "crbUser",
	}

	group = &permissionIndex{
		clusterRoleLister:   client.ClusterRoles("").Controller().Lister(),
		roleLister:          client.Roles("").Controller().Lister(),
		crbIndexer:          client.ClusterRoleBindings("").Controller().Informer().GetIndexer(),
		rbIndexer:           client.RoleBindings("").Controller().Informer().GetIndexer(),
		roleIndexKey:        "rbGroup",
		clusterRoleIndexKey: "crbGroup",
	}

	return
}

func clusterRoleBindingIndexer(kind string, obj interface{}) ([]string, error) {
	var result []string
	crb := obj.(*rbacv1.ClusterRoleBinding)
	for _, subject := range crb.Subjects {
		if subject.Kind == kind {
			result = append(result, subject.Name)
		}
	}
	return result, nil
}

func roleBindingIndexer(kind string, obj interface{}) ([]string, error) {
	var result []string
	crb := obj.(*rbacv1.RoleBinding)
	for _, subject := range crb.Subjects {
		if subject.Kind == kind {
			result = append(result, subject.Name)
		}
	}
	return result, nil
}

type permissionIndex struct {
	clusterRoleLister   v1.ClusterRoleLister
	roleLister          v1.RoleLister
	crbIndexer          cache.Indexer
	rbIndexer           cache.Indexer
	roleIndexKey        string
	clusterRoleIndexKey string
}

func (p *permissionIndex) get(name, apiGroup, resource string) []ListPermission {
	var result []ListPermission

	for _, binding := range p.getRoleBindings(name) {
		if binding.RoleRef.APIGroup != rbacGroup {
			continue
		}

		result = p.filterPermissions(result, binding.Namespace, binding.RoleRef.Kind, binding.RoleRef.Name, apiGroup, resource)
	}

	for _, binding := range p.getClusterRoleBindings(name) {
		if binding.RoleRef.APIGroup != rbacGroup {
			continue
		}
		result = p.filterPermissions(result, "*", binding.RoleRef.Kind, binding.RoleRef.Name, apiGroup, resource)
	}

	return result
}

func (p *permissionIndex) filterPermissions(result []ListPermission, namespace, kind, name, apiGroup, resource string) []ListPermission {
	for _, rule := range p.getRules(namespace, kind, name) {
		if !matches(rule.APIGroups, apiGroup) || !matches(rule.Resources, resource) {
			continue
		}

		nsForResourceNameGets := namespace
		if namespace == "*" {
			nsForResourceNameGets = ""
		}
		for _, verb := range rule.Verbs {
			switch verb {
			case "*":
				fallthrough
			case "list":
				result = append(result, ListPermission{
					Namespace: namespace,
					Name:      "*",
				})
			case "get":
				for _, resourceName := range rule.ResourceNames {
					result = append(result, ListPermission{
						Namespace: nsForResourceNameGets,
						Name:      resourceName,
					})
				}
			}
		}
	}

	return result
}

func (p *permissionIndex) getClusterRoleBindings(name string) []*rbacv1.ClusterRoleBinding {
	var result []*rbacv1.ClusterRoleBinding

	objs, err := p.crbIndexer.ByIndex(p.clusterRoleIndexKey, name)
	if err != nil {
		return result
	}

	for _, obj := range objs {
		result = append(result, obj.(*rbacv1.ClusterRoleBinding))
	}

	return result
}

func (p *permissionIndex) getRoleBindings(name string) []*rbacv1.RoleBinding {
	var result []*rbacv1.RoleBinding

	objs, err := p.rbIndexer.ByIndex(p.roleIndexKey, name)
	if err != nil {
		return result
	}

	for _, obj := range objs {
		result = append(result, obj.(*rbacv1.RoleBinding))
	}

	return result
}

func (p *permissionIndex) getRules(namespace, kind, name string) []rbacv1.PolicyRule {
	switch kind {
	case "ClusterRole":
		role, err := p.clusterRoleLister.Get("", name)
		if err != nil {
			return nil
		}
		return role.Rules
	case "Role":
		role, err := p.roleLister.Get(namespace, name)
		if err != nil {
			return nil
		}
		return role.Rules
	}

	return nil
}

func matches(parts []string, val string) bool {
	for _, value := range parts {
		if value == "*" {
			return true
		}
		if value == val {
			return true
		}
	}
	return false
}
