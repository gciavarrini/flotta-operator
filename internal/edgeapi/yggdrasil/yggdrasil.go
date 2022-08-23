package yggdrasil

import (
	"context"
	"encoding/json"
	"github.com/project-flotta/flotta-operator/internal/labels"
	"github.com/project-flotta/flotta-operator/internal/repository/edgeconfig"
	"github.com/project-flotta/flotta-operator/internal/repository/playbookexecution"
	"net/http"
	"strings"
	"time"

	"github.com/go-openapi/runtime/middleware"
	"github.com/go-openapi/strfmt"
	"github.com/google/uuid"
	"go.uber.org/zap"
	"k8s.io/apimachinery/pkg/api/errors"

	"github.com/project-flotta/flotta-operator/internal/common/metrics"
	backendapi "github.com/project-flotta/flotta-operator/internal/edgeapi/backend"
	"github.com/project-flotta/flotta-operator/models"
	"github.com/project-flotta/flotta-operator/pkg/mtls"
	apioperations "github.com/project-flotta/flotta-operator/restapi/operations"
	"github.com/project-flotta/flotta-operator/restapi/operations/yggdrasil"
	operations "github.com/project-flotta/flotta-operator/restapi/operations/yggdrasil"
)

const (
	YggdrasilRegisterAuth                                = 1
	YggdrasilCompleteAuth                                = 0
	AuthzKey                         mtls.RequestAuthKey = "APIAuthzkey"
	YggrasilAPIRegistrationOperation                     = "PostDataMessageForDevice"
)

type Handler struct {
	backend          backendapi.EdgeDeviceBackend
	edgeConfigRepository              edgeconfig.Repository
	playbookExecutionRepository       playbookexecution.Repository
	deviceSetRepository               edgedeviceset.Repository
	initialNamespace string
	metrics          metrics.Metrics
	heartbeatHandler *RetryingDelegatingHandler
	mtlsConfig       *mtls.TLSConfig
	logger           *zap.SugaredLogger
}

func NewYggdrasilHandler(initialNamespace string, metrics metrics.Metrics, mtlsConfig *mtls.TLSConfig, logger *zap.SugaredLogger,
	backend backendapi.EdgeDeviceBackend) *Handler {
	return &Handler{
		initialNamespace: initialNamespace,
		metrics:          metrics,
		heartbeatHandler: NewRetryingDelegatingHandler(backend),
		mtlsConfig:       mtlsConfig,
		logger:           logger,
		backend:          backend,
	}
}

func IsOwnDevice(ctx context.Context, deviceID string) bool {
	if deviceID == "" {
		return false
	}

	val, ok := ctx.Value(AuthzKey).(mtls.RequestAuthVal)
	if !ok {
		return false
	}
	return val.CommonName == strings.ToLower(deviceID)
}

// GetAuthType returns the kind of the authz that need to happen on the API call, the options are:
// YggdrasilCompleteAuth: need to be a valid client certificate and not expired.
// YggdrasilRegisterAuth: it is only valid for registering action.
func (h *Handler) GetAuthType(r *http.Request, api *apioperations.FlottaManagementAPI) int {
	res := YggdrasilCompleteAuth
	if api == nil {
		return res
	}

	route, _, matches := api.Context().RouteInfo(r)
	if !matches {
		return res
	}

	if route != nil && route.Operation != nil {
		if route.Operation.ID == YggrasilAPIRegistrationOperation {
			return YggdrasilRegisterAuth
		}
	}
	return res
}

func (h *Handler) getNamespace(ctx context.Context) string {
	ns := h.initialNamespace

	val, ok := ctx.Value(AuthzKey).(mtls.RequestAuthVal)
	if !ok {
		return ns
	}

	if val.Namespace != "" {
		return val.Namespace
	}
	return ns
}

func (h *Handler) GetControlMessageForDevice(ctx context.Context, params yggdrasil.GetControlMessageForDeviceParams) middleware.Responder {
	deviceID := params.DeviceID
	if !IsOwnDevice(ctx, deviceID) {
		h.metrics.IncEdgeDeviceInvalidOwnerCounter()
		return operations.NewGetControlMessageForDeviceForbidden()
	}
	logger := h.logger.With("DeviceID", deviceID)

	regStatus, err := h.backend.GetRegistrationStatus(ctx, deviceID, h.getNamespace(ctx))
	if err != nil {
		if errors.IsNotFound(err) {
			logger.Info("edge device is not found")
			return operations.NewGetControlMessageForDeviceNotFound()
		}
		logger.With("err", err).Error("failed to get edge device")
		return operations.NewGetControlMessageForDeviceInternalServerError()
	}

	if regStatus == backendapi.Unregistered {
		h.metrics.IncEdgeDeviceUnregistration()
		message := h.createDisconnectCommand()
		return operations.NewGetControlMessageForDeviceOK().WithPayload(message)
	}

	return operations.NewGetControlMessageForDeviceOK()
}

func (h *Handler) GetDataMessageForDevice(ctx context.Context, params yggdrasil.GetDataMessageForDeviceParams) middleware.Responder {
	deviceID := params.DeviceID
	if !IsOwnDevice(ctx, deviceID) {
		h.metrics.IncEdgeDeviceInvalidOwnerCounter()
		return operations.NewGetDataMessageForDeviceForbidden()
	}
	logger := h.logger.With("DeviceID", deviceID)

	dc, err := h.backend.GetConfiguration(ctx, deviceID, h.getNamespace(ctx))
	if err != nil {
		if errors.IsNotFound(err) {
			logger.Info("edge device is not found")
			return operations.NewGetDataMessageForDeviceNotFound()
		}
		logger.With("err", err).Error("failed to get edge device configuration")
		return operations.NewGetDataMessageForDeviceInternalServerError()
	}
	// var deviceSet *v1alpha1.EdgeDeviceSet
	if deviceSetName, ok := edgeDevice.Labels["flotta/member-of"]; ok {
		logger.V(1).Info("Device uses EdgeDeviceSet", "edgeDeviceSet", deviceSetName)
		if labels.IsConfigLabel(deviceSetName) {
			playbookExecution, err := h.playbookExecutionRepository.Read(ctx, deviceSetName, h.initialNamespace)
			if err != nil {
				if errors.IsNotFound(err) {
					logger.Info("playbook execution is not found")
					edgeConfig, err := h.edgeConfigRepository.Read(ctx, deviceSetName, h.initialNamespace)
					if err != nil {
						if errors.IsNotFound(err) {
							logger.Info("edge config is not found")
							return operations.NewGetDataMessageForDeviceNotFound()
						}
						logger.Error(err, "failed to get edge config")
						return operations.NewGetDataMessageForDeviceInternalServerError()
					}
					err = createPlaybookExecution(ctx, edgeConfig, edgeDevice, h.playbookExecutionRepository)
					if err != nil {
						logger.Error(err, "failed to create playbook execution")
						return operations.NewGetDataMessageForDeviceInternalServerError()
					}
				} else {
					logger.Error(err, "failed to get playbook execution")
					return operations.NewGetDataMessageForDeviceInternalServerError()
				}
			}
			if playbookExecution == nil {

			}
		}
		// var err error
		// deviceSet, err = h.deviceSetRepository.Read(ctx, deviceSetName, edgeDevice.Namespace)
		// if err != nil {
		// 	logger.Info("Cannot load EdgeDeviceSet", "edgeDeviceSet", deviceSetName)
		// 	deviceSet = nil
		// }
	}

	// h.edgeConfigRepository.ListByLabel()

	// h.deviceRepository.ListForEdgeConfig(ctx)
	// edgeConfig, err := h.edgeConfigRepository.Read(ctx, deviceID, h.initialNamespace)
	// if err != nil {
	// 	if errors.IsNotFound(err) {
	// 		logger.Info("edge config is not found")
	// 		return operations.NewGetDataMessageForDeviceNotFound()
	// 	}
	// }

	// logger.Info("edge config found", "edgeConfig", edgeConfig.Name)

	// TODO: Network optimization: Decide whether there is a need to return any payload based on difference between last applied configuration and current state in the cluster.
	message := models.Message{
		Type:      models.MessageTypeData,
		Directive: "device",
		MessageID: uuid.New().String(),
		Version:   1,
		Sent:      strfmt.DateTime(time.Now()),
		Content:   *dc,
	}
	return operations.NewGetDataMessageForDeviceOK().WithPayload(&message)
}

func createPlaybookExecution(ctx context.Context, edgeConfig *v1alpha1.EdgeConfig, edgeDevice *v1alpha1.EdgeDevice, playbookExecutionRepo playbookexecution.Repository) error {
	playbookExecution := &v1alpha1.PlaybookExecution{}

	playbookExecution.Spec.Playbook = edgeConfig.Spec.EdgePlaybook.Playbooks[0] //TODO: for each
	playbookExecution.Spec.ExecutionAttempt = 0

	err := playbookExecutionRepo.Create(ctx, playbookExecution)
	if err != nil {
		return err
	}

	return nil
}

func (h *Handler) PostControlMessageForDevice(ctx context.Context, params yggdrasil.PostControlMessageForDeviceParams) middleware.Responder {
	deviceID := params.DeviceID
	if !IsOwnDevice(ctx, deviceID) {
		h.metrics.IncEdgeDeviceInvalidOwnerCounter()
		return operations.NewPostDataMessageForDeviceForbidden()
	}
	return operations.NewPostControlMessageForDeviceOK()
}

func (h *Handler) PostDataMessageForDevice(ctx context.Context, params yggdrasil.PostDataMessageForDeviceParams) middleware.Responder {
	deviceID := params.DeviceID
	logger := h.logger.With("DeviceID", deviceID)
	msg := params.Message
	switch msg.Directive {
	case "registration", "enrolment":
		break
	default:
		if !IsOwnDevice(ctx, deviceID) {
			h.metrics.IncEdgeDeviceInvalidOwnerCounter()
			return operations.NewPostDataMessageForDeviceForbidden()
		}
	}
	switch msg.Directive {
	case "heartbeat":
		hb := models.Heartbeat{}
		contentJson, _ := json.Marshal(msg.Content)
		err := json.Unmarshal(contentJson, &hb)
		if err != nil {
			return operations.NewPostDataMessageForDeviceBadRequest()
		}
		err = h.heartbeatHandler.Process(ctx, deviceID, h.getNamespace(ctx), &hb)
		if err != nil {
			if errors.IsNotFound(err) {
				logger.Debug("Device not found")
				return operations.NewPostDataMessageForDeviceNotFound()
			}
			logger.With("err", err).Error("Device not found")
			return operations.NewPostDataMessageForDeviceInternalServerError()
		}
		h.metrics.RecordEdgeDevicePresence(h.getNamespace(ctx), deviceID)
	case "enrolment":
		contentJson, _ := json.Marshal(msg.Content)
		enrolmentInfo := models.EnrolmentInfo{}
		err := json.Unmarshal(contentJson, &enrolmentInfo)
		if err != nil {
			return operations.NewPostDataMessageForDeviceBadRequest()
		}
		logger.With("content", enrolmentInfo).Debug("received enrolment info")
		targetNamespace := h.initialNamespace
		if enrolmentInfo.TargetNamespace != nil {
			targetNamespace = *enrolmentInfo.TargetNamespace
		}
		alreadyCreated, err := h.backend.Enrol(ctx, deviceID, targetNamespace, &enrolmentInfo)
		if err != nil {
			return operations.NewPostDataMessageForDeviceBadRequest()
		}

		if alreadyCreated {
			return operations.NewPostDataMessageForDeviceAlreadyReported()
		}

		return operations.NewPostDataMessageForDeviceOK()
	case "registration":
		// register new edge device
		contentJson, _ := json.Marshal(msg.Content)
		registrationInfo := models.RegistrationInfo{}
		err := json.Unmarshal(contentJson, &registrationInfo)
		if err != nil {
			return operations.NewPostDataMessageForDeviceBadRequest()
		}
		logger.With("content", registrationInfo).Debug("received registration info")
		res := models.MessageResponse{
			Directive: msg.Directive,
			MessageID: msg.MessageID,
		}
		content := models.RegistrationResponse{}
		ns := h.getNamespace(ctx)

		ns, err = h.backend.GetTargetNamespace(ctx, deviceID, ns, IsOwnDevice(ctx, deviceID))
		if err != nil {
			logger.With("err", err).Error("can't get target namespace for a device")
			if !errors.IsNotFound(err) {
				h.metrics.IncEdgeDeviceFailedRegistration()
				return operations.NewPostDataMessageForDeviceInternalServerError()
			}

			if _, ok := err.(*backendapi.NotApproved); !ok {
				h.metrics.IncEdgeDeviceFailedRegistration()
			}
			return operations.NewPostDataMessageForDeviceNotFound()
		}
		cert, err := h.mtlsConfig.SignCSR(registrationInfo.CertificateRequest, deviceID, ns)
		if err != nil {
			return operations.NewPostDataMessageForDeviceBadRequest()
		}
		content.Certificate = string(cert)

		res.Content = content
		err = h.backend.Register(ctx, deviceID, ns, &registrationInfo)

		if err != nil {
			logger.With("err", err).Error("cannot finalize device registration")
			h.metrics.IncEdgeDeviceFailedRegistration()
			return operations.NewPostDataMessageForDeviceInternalServerError()
		}

		h.metrics.IncEdgeDeviceSuccessfulRegistration()
		return operations.NewPostDataMessageForDeviceOK().WithPayload(&res)
	case "ansible":
		ns := h.getNamespace(ctx)

		edgeDevice, err := h.edgeDeviceRepository.Read(ctx, deviceID, ns)
		if err != nil {
			if !errors.IsNotFound(err) {
				h.metrics.IncEdgeDeviceFailedRegistration()
				return operations.NewPostDataMessageForDeviceInternalServerError()
			}
			return operations.NewPostDataMessageForDeviceNotFound()
		}

		for labelName, labelValue := range edgeDevice.ObjectMeta.Labels {
			if labels.IsEdgeConfigLabel(labelName) { //FIXME: what if the are multiple config labels?

				playbookExecution, err := h.playbookExecutionRepository.Read(ctx, labelValue, h.getNamespace(ctx))
				if err != nil {
					if errors.IsNotFound(err) {
						return operations.NewGetDataMessageForDeviceNotFound()
					}
					return operations.NewGetDataMessageForDeviceInternalServerError()
				}
				if playbookExecution == nil {
					return operations.NewGetDataMessageForDeviceInternalServerError()
				}

				message := models.Message{
					Type:      models.MessageTypeData,
					Metadata:  map[string]string{"ansible-playbook": "true"},
					Directive: "ansible",
					MessageID: uuid.New().String(),
					Version:   1,
					Sent:      strfmt.DateTime(time.Now()),
					Content:   playbookExecution,
				}
				return operations.NewGetDataMessageForDeviceOK().WithPayload(&message)
			}
		}
	default:
		logger.With("message", msg).Info("received unknown message")
		return operations.NewPostDataMessageForDeviceBadRequest()
	}
	return operations.NewPostDataMessageForDeviceOK()
}

func (h *Handler) createDisconnectCommand() *models.Message {
	command := struct {
		Command   string            `json:"command"`
		Arguments map[string]string `json:"arguments"`
	}{
		Command: "disconnect",
	}

	return &models.Message{
		Type:      models.MessageTypeCommand,
		MessageID: uuid.New().String(),
		Version:   1,
		Sent:      strfmt.DateTime(time.Now()),
		Content:   command,
	}
}
