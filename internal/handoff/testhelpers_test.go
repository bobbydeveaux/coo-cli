package handoff

import (
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
)

// makeConcept builds an *unstructured.Unstructured COOConcept for use in tests.
func makeConcept(name, phase, tier, rawConcept string, projects []string) *unstructured.Unstructured {
	var projs []interface{}
	for _, p := range projects {
		projs = append(projs, p)
	}
	obj := map[string]interface{}{
		"apiVersion": cooAPIGroup + "/" + cooAPIVersion,
		"kind":       "COOConcept",
		"metadata": map[string]interface{}{
			"name":      name,
			"namespace": cooSystem,
		},
		"spec": map[string]interface{}{
			"rawConcept":       rawConcept,
			"affectedProjects": projs,
		},
		"status": map[string]interface{}{
			"phase": phase,
			"complexityAssessment": map[string]interface{}{
				"tier": tier,
			},
		},
	}
	u := &unstructured.Unstructured{Object: obj}
	return u
}

// makePlan builds an *unstructured.Unstructured COOPlan for use in tests.
func makePlan(name, ns, prURL string, epicCount, featureCount, issueCount int64) *unstructured.Unstructured {
	obj := map[string]interface{}{
		"apiVersion": cooAPIGroup + "/" + cooAPIVersion,
		"kind":       "COOPlan",
		"metadata": map[string]interface{}{
			"name":      name,
			"namespace": ns,
		},
		"spec": map[string]interface{}{
			"artifacts": map[string]interface{}{
				"prd": "docs/PRD.md",
				"hld": "docs/HLD.md",
			},
		},
		"status": map[string]interface{}{
			"planningPRURL": prURL,
			"epicCount":     epicCount,
			"featureCount":  featureCount,
			"issueCount":    issueCount,
		},
	}
	return &unstructured.Unstructured{Object: obj}
}

// makeTask builds an *unstructured.Unstructured COOTask for use in tests.
func makeTask(name, ns, worker, priority, phase string, prNumber int64) *unstructured.Unstructured {
	return &unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": cooAPIGroup + "/" + cooAPIVersion,
			"kind":       "COOTask",
			"metadata": map[string]interface{}{
				"name":      name,
				"namespace": ns,
			},
			"spec": map[string]interface{}{
				"worker":   worker,
				"priority": priority,
			},
			"status": map[string]interface{}{
				"phase":    phase,
				"prNumber": prNumber,
			},
		},
	}
}
