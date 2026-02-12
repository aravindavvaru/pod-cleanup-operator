package controller

import (
	"context"
	"fmt"
	"time"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"

	"github.com/robfig/cron/v3"

	cleanupv1 "github.com/aravindavvaru/pod-cleanup-operator/api/v1"
)

// PodCleanupPolicyReconciler reconciles a PodCleanupPolicy object
type PodCleanupPolicyReconciler struct {
	client.Client
	Scheme *runtime.Scheme
}

//+kubebuilder:rbac:groups=cleanup.k8s.io,resources=podcleanuppolicies,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=cleanup.k8s.io,resources=podcleanuppolicies/status,verbs=get;update;patch
//+kubebuilder:rbac:groups=cleanup.k8s.io,resources=podcleanuppolicies/finalizers,verbs=update
//+kubebuilder:rbac:groups="",resources=pods,verbs=get;list;watch;delete
//+kubebuilder:rbac:groups="",resources=namespaces,verbs=get;list;watch

// Reconcile implements the main reconciliation loop for PodCleanupPolicy.
// It evaluates the cleanup schedule, selects matching pods, and deletes them
// (or logs them in dry-run mode).
func (r *PodCleanupPolicyReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	logger := log.FromContext(ctx)

	policy := &cleanupv1.PodCleanupPolicy{}
	if err := r.Get(ctx, req.NamespacedName, policy); err != nil {
		if errors.IsNotFound(err) {
			return ctrl.Result{}, nil
		}
		return ctrl.Result{}, err
	}

	// If a cron schedule is configured, check whether it is time to run.
	if policy.Spec.Schedule != "" {
		parser := cron.NewParser(cron.Minute | cron.Hour | cron.Dom | cron.Month | cron.Dow)
		schedule, err := parser.Parse(policy.Spec.Schedule)
		if err != nil {
			logger.Error(err, "Invalid cron schedule", "schedule", policy.Spec.Schedule)
			r.setCondition(policy, "Ready", metav1.ConditionFalse, "InvalidSchedule",
				fmt.Sprintf("Cannot parse cron schedule %q: %v", policy.Spec.Schedule, err))
			_ = r.Status().Update(ctx, policy)
			// Do not requeue; the spec needs to be fixed first.
			return ctrl.Result{}, nil
		}

		var lastRun time.Time
		if policy.Status.LastRunTime != nil {
			lastRun = policy.Status.LastRunTime.Time
		}

		nextRun := schedule.Next(lastRun)
		now := time.Now()
		if nextRun.After(now) {
			requeueAfter := nextRun.Sub(now)
			logger.Info("Next cleanup scheduled", "nextRun", nextRun, "requeueAfter", requeueAfter)
			return ctrl.Result{RequeueAfter: requeueAfter}, nil
		}
	}

	// Execute the cleanup.
	deleted, err := r.runCleanup(ctx, policy)
	if err != nil {
		r.setCondition(policy, "Ready", metav1.ConditionFalse, "CleanupFailed", err.Error())
	} else {
		msg := fmt.Sprintf("Cleanup completed; %d pod(s) deleted", deleted)
		if policy.Spec.DryRun {
			msg = fmt.Sprintf("DryRun cleanup completed; %d pod(s) would be deleted", deleted)
		}
		r.setCondition(policy, "Ready", metav1.ConditionTrue, "CleanupSucceeded", msg)
	}

	now := metav1.Now()
	policy.Status.LastRunTime = &now
	policy.Status.LastRunPodsDeleted = int32(deleted)
	if !policy.Spec.DryRun {
		policy.Status.PodsDeleted += int64(deleted)
	}

	if statusErr := r.Status().Update(ctx, policy); statusErr != nil {
		logger.Error(statusErr, "Failed to update PodCleanupPolicy status")
		return ctrl.Result{}, statusErr
	}

	if err != nil {
		return ctrl.Result{}, err
	}

	// Schedule the next run when a cron schedule is configured.
	if policy.Spec.Schedule != "" {
		parser := cron.NewParser(cron.Minute | cron.Hour | cron.Dom | cron.Month | cron.Dow)
		schedule, _ := parser.Parse(policy.Spec.Schedule)
		nextRun := schedule.Next(time.Now())
		return ctrl.Result{RequeueAfter: time.Until(nextRun)}, nil
	}

	return ctrl.Result{}, nil
}

// runCleanup iterates over all target namespaces and deletes matching pods.
func (r *PodCleanupPolicyReconciler) runCleanup(ctx context.Context, policy *cleanupv1.PodCleanupPolicy) (int, error) {
	logger := log.FromContext(ctx)

	namespaces, err := r.getTargetNamespaces(ctx, policy)
	if err != nil {
		return 0, fmt.Errorf("listing target namespaces: %w", err)
	}

	total := 0
	for _, ns := range namespaces {
		count, err := r.cleanupPodsInNamespace(ctx, policy, ns)
		if err != nil {
			logger.Error(err, "Error cleaning pods in namespace", "namespace", ns)
			continue
		}
		total += count
	}

	logger.Info("Cleanup run finished", "podsAffected", total, "dryRun", policy.Spec.DryRun)
	return total, nil
}

// getTargetNamespaces returns the list of namespace names that the policy applies to.
func (r *PodCleanupPolicyReconciler) getTargetNamespaces(ctx context.Context, policy *cleanupv1.PodCleanupPolicy) ([]string, error) {
	nsList := &corev1.NamespaceList{}

	if policy.Spec.NamespaceSelector == nil {
		if err := r.List(ctx, nsList); err != nil {
			return nil, err
		}
	} else {
		selector, err := metav1.LabelSelectorAsSelector(policy.Spec.NamespaceSelector)
		if err != nil {
			return nil, fmt.Errorf("invalid namespaceSelector: %w", err)
		}
		if err := r.List(ctx, nsList, &client.ListOptions{LabelSelector: selector}); err != nil {
			return nil, err
		}
	}

	names := make([]string, 0, len(nsList.Items))
	for _, ns := range nsList.Items {
		names = append(names, ns.Name)
	}
	return names, nil
}

// cleanupPodsInNamespace lists pods in the given namespace and deletes those that
// match the policy criteria.
func (r *PodCleanupPolicyReconciler) cleanupPodsInNamespace(ctx context.Context, policy *cleanupv1.PodCleanupPolicy, namespace string) (int, error) {
	logger := log.FromContext(ctx)

	listOpts := []client.ListOption{client.InNamespace(namespace)}
	if policy.Spec.PodSelector != nil {
		selector, err := metav1.LabelSelectorAsSelector(policy.Spec.PodSelector)
		if err != nil {
			return 0, fmt.Errorf("invalid podSelector: %w", err)
		}
		listOpts = append(listOpts, client.MatchingLabelsSelector{Selector: selector})
	}

	podList := &corev1.PodList{}
	if err := r.List(ctx, podList, listOpts...); err != nil {
		return 0, err
	}

	deleted := 0
	for i := range podList.Items {
		pod := &podList.Items[i]
		if !r.shouldDeletePod(policy, pod) {
			continue
		}

		podAge := time.Since(pod.CreationTimestamp.Time).Round(time.Second)
		if policy.Spec.DryRun {
			logger.Info("DryRun: would delete pod",
				"namespace", pod.Namespace,
				"pod", pod.Name,
				"phase", pod.Status.Phase,
				"age", podAge,
			)
			deleted++
			continue
		}

		logger.Info("Deleting pod",
			"namespace", pod.Namespace,
			"pod", pod.Name,
			"phase", pod.Status.Phase,
			"age", podAge,
		)
		if err := r.Delete(ctx, pod); err != nil && !errors.IsNotFound(err) {
			logger.Error(err, "Failed to delete pod", "pod", pod.Name, "namespace", pod.Namespace)
			continue
		}
		deleted++
	}

	return deleted, nil
}

// shouldDeletePod returns true when the pod satisfies all criteria defined in the policy.
func (r *PodCleanupPolicyReconciler) shouldDeletePod(policy *cleanupv1.PodCleanupPolicy, pod *corev1.Pod) bool {
	// Filter by pod phase, if specified.
	if len(policy.Spec.PodStatuses) > 0 {
		matched := false
		for _, phase := range policy.Spec.PodStatuses {
			if pod.Status.Phase == phase {
				matched = true
				break
			}
		}
		if !matched {
			return false
		}
	}

	// Filter by age, if specified.
	if policy.Spec.MaxAge != "" {
		maxAge, err := time.ParseDuration(policy.Spec.MaxAge)
		if err != nil {
			// Invalid maxAge – skip this pod rather than panic.
			return false
		}
		if time.Since(pod.CreationTimestamp.Time) < maxAge {
			return false
		}
	}

	return true
}

// setCondition updates or appends a condition on the policy status.
func (r *PodCleanupPolicyReconciler) setCondition(policy *cleanupv1.PodCleanupPolicy, condType string, status metav1.ConditionStatus, reason, message string) {
	cond := metav1.Condition{
		Type:               condType,
		Status:             status,
		Reason:             reason,
		Message:            message,
		LastTransitionTime: metav1.Now(),
		ObservedGeneration: policy.Generation,
	}
	for i, existing := range policy.Status.Conditions {
		if existing.Type == condType {
			if existing.Status != status {
				policy.Status.Conditions[i] = cond
			} else {
				// Only update message/reason; keep original transition time.
				policy.Status.Conditions[i].Reason = reason
				policy.Status.Conditions[i].Message = message
				policy.Status.Conditions[i].ObservedGeneration = policy.Generation
			}
			return
		}
	}
	policy.Status.Conditions = append(policy.Status.Conditions, cond)
}

// SetupWithManager registers the controller with the manager.
func (r *PodCleanupPolicyReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&cleanupv1.PodCleanupPolicy{}).
		Complete(r)
}
