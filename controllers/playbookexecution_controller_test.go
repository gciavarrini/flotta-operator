package controllers_test

import (
	"context"

	"github.com/golang/mock/gomock"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	"github.com/project-flotta/flotta-operator/api/v1alpha1"
	"github.com/project-flotta/flotta-operator/controllers"
	"github.com/project-flotta/flotta-operator/internal/common/repository/edgedevice"
	"github.com/project-flotta/flotta-operator/internal/common/repository/playbookexecution"
)

var _ = Describe("PlaybookExecution controller", func() {
	var (
		playbookExecutionReconciler *controllers.PlaybookExecutionReconciler
		err                         error
		cancelContext               context.CancelFunc
		signalContext               context.Context
		req                         ctrl.Request

		playbookExecutionRepoMock *playbookexecution.MockRepository
		edgeDeviceRepoMock        *edgedevice.MockRepository
		// playbookExecRepoMock      *playbookexecution.MockRepository
		k8sManager manager.Manager
		// playbookExecutionName = "playbookexecution-test"
		namespace = "test"
	)

	BeforeEach(func() {
		GinkgoRecover()
		k8sManager = getK8sManager(cfg)
		mockCtrl := gomock.NewController(GinkgoT())

		playbookExecutionRepoMock = playbookexecution.NewMockRepository(mockCtrl)
		edgeDeviceRepoMock = edgedevice.NewMockRepository(mockCtrl)

		playbookExecutionReconciler = &controllers.PlaybookExecutionReconciler{
			Client:                      k8sClient,
			Scheme:                      k8sManager.GetScheme(),
			EdgeDeviceRepository:        edgeDeviceRepoMock,
			PlaybookExecutionRepository: playbookExecutionRepoMock,
		}
		err = playbookExecutionReconciler.SetupWithManager(k8sManager)
		Expect(err).ToNot(HaveOccurred())

		signalContext, cancelContext = context.WithCancel(context.TODO())
		go func() {
			err = k8sManager.Start(signalContext)
			Expect(err).ToNot(HaveOccurred())
		}()

		req = ctrl.Request{
			NamespacedName: types.NamespacedName{
				Name:      "test",
				Namespace: namespace,
			},
		}

	})
	AfterEach(func() {
		cancelContext()
	})

	Context("Reconcile", func() {
		BeforeEach(func() {

			playbookExecutionReconciler = &controllers.PlaybookExecutionReconciler{
				Client:                      k8sClient,
				Scheme:                      k8sManager.GetScheme(),
				EdgeDeviceRepository:        edgeDeviceRepoMock,
				PlaybookExecutionRepository: playbookExecutionRepoMock,
			}
		})

		// getPlaybookExecution := func(name string) *v1alpha1.PlaybookExecution {
		// 	return &v1alpha1.PlaybookExecution{
		// 		ObjectMeta: v1.ObjectMeta{
		// 			Name:      name,
		// 			Namespace: namespace,
		// 		},
		// 		Spec: v1alpha1.PlaybookExecutionSpec{
		// 			Playbook: v1alpha1.Playbook{
		// 				Content: []byte("test"),
		// 			},
		// 		},
		// 	}
		// }
		It("PlaybookExecution does not exists on CRD", func() {
			// given
			returnErr := errors.NewNotFound(schema.GroupResource{Group: "", Resource: "notfound"}, "notfound")
			playbookExecutionRepoMock.EXPECT().
				Read(gomock.Any(), req.Name, req.Namespace).
				Return(nil, returnErr).
				Times(1)
			res, err := playbookExecutionReconciler.Reconcile(context.TODO(), req)

			// then
			Expect(err).NotTo(HaveOccurred())
			Expect(res).To(Equal(reconcile.Result{Requeue: false, RequeueAfter: 0}))
		})
		It("Cannot get playbookexecution", func() {
			// given
			returnErr := errors.NewNotFound(schema.GroupResource{Group: "", Resource: "notfound"}, "notfound")

			playbookExecutionRepoMock.EXPECT().
				Read(gomock.Any(), gomock.Any(), namespace).
				Return(nil, returnErr).
				Times(1)
			// when
			res, err := playbookExecutionReconciler.Reconcile(context.TODO(), req)

			// then
			Expect(err).To(HaveOccurred())
			Expect(res).To(Equal(reconcile.Result{Requeue: true, RequeueAfter: 0}))
		})
	})

	// Context("edgeDevice selection", func() {
	// 	var (
	// 		playbookExecutionData *v1alpha1.PlaybookExecution
	// 		// device                *v1alpha1.EdgeDevice
	// 		namespace = "default"
	// 	)
	// 	// getDevice := func(name string) *v1alpha1.EdgeDevice {
	// 	// 	return &v1alpha1.EdgeDevice{
	// 	// 		ObjectMeta: v1.ObjectMeta{
	// 	// 			Name:      name,
	// 	// 			Namespace: namespace,
	// 	// 		},
	// 	// 		Spec: v1alpha1.EdgeDeviceSpec{
	// 	// 			RequestTime: &v1.Time{},
	// 	// 			Heartbeat:   &v1alpha1.HeartbeatConfiguration{},
	// 	// 		},
	// 	// 		Status: v1alpha1.EdgeDeviceStatus{
	// 	// 			PlaybookExecutions: []v1alpha1.PlaybookExecution{
	// 	// 				{
	// 	// 					ObjectMeta: v1.ObjectMeta{
	// 	// 						Name:       playbookExecutionName,
	// 	// 						Namespace:  namespace,
	// 	// 						Finalizers: []string{controllers.YggdrasilDeviceReferenceFinalizer},
	// 	// 					},
	// 	// 					Spec: v1alpha1.PlaybookExecutionSpec{
	// 	// 						Playbook: v1alpha1.Playbook{
	// 	// 							Content: []byte("test"),
	// 	// 						},
	// 	// 					},
	// 	// 				},
	// 	// 			},
	// 	// 		},
	// 	// 	}
	// 	// }
	// 	BeforeEach(func() {
	// 		playbookExecutionData = &v1alpha1.PlaybookExecution{
	// 			ObjectMeta: v1.ObjectMeta{
	// 				Name:       playbookExecutionName,
	// 				Namespace:  namespace,
	// 				Finalizers: []string{controllers.YggdrasilDeviceReferenceFinalizer},
	// 			},
	// 			Spec: v1alpha1.PlaybookExecutionSpec{
	// 				Playbook: v1alpha1.Playbook{
	// 					Content: []byte("test"),
	// 				},
	// 			},
	// 		}
	// 		playbookExecutionRepoMock.EXPECT().Read(gomock.Any(), gomock.Any(), gomock.Any()).
	// 			Return(playbookExecutionData, nil).Times(1)

	// 		// device = getDevice("testdevice")
	// 	})

	// })
})

func getExpectedPlaybookExecution(ctx context.Context, objectKey client.ObjectKey) v1alpha1.PlaybookExecution {
	var ed v1alpha1.PlaybookExecution
	err := k8sClient.Get(ctx, objectKey, &ed)
	ExpectWithOffset(1, err).ToNot(HaveOccurred())
	return ed
}
