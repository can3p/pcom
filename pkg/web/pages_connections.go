package web

import (
	"fmt"
	"time"

	"github.com/can3p/pcom/pkg/auth"
	"github.com/can3p/pcom/pkg/model/core"
	"github.com/can3p/pcom/pkg/userops"
	"github.com/gin-gonic/gin"
	"github.com/samber/lo"
	"github.com/samber/mo"
	"github.com/volatiletech/sqlboiler/v4/boil"
	"github.com/volatiletech/sqlboiler/v4/queries/qm"
)

type MediationRequest struct {
	Requester *core.User
	Target    *core.User
	Request   *core.UserConnectionMediationRequest
}

type MediationResult struct {
	Mediation *core.UserConnectionMediator
	Mediator  *core.User
}

type ConnectionRequest struct {
	Requester  *core.User
	Request    *core.UserConnectionMediationRequest
	Mediations []*MediationResult
}

type Draft struct {
	PostID        string
	Subject       string
	LastUpdatedAt time.Time
}

type ControlsPage struct {
	*BasePage
	DirectConnections       core.UserSlice
	SecondDegreeConnections core.UserSlice
	WhitelistedConnections  core.UserSlice
	MediationRequests       []*MediationRequest
	ConnectionRequests      []*ConnectionRequest
	Drafts                  []*Draft
}

func Controls(ctx *gin.Context, db boil.ContextExecutor, userData *auth.UserData) mo.Result[*ControlsPage] {
	userID := userData.DBUser.ID
	directUserIDs, secondDegreeUserIDs, _, err := userops.GetDirectAndSecondDegreeUserIDs(ctx, db, userID)

	if err != nil {
		return mo.Err[*ControlsPage](err)
	}

	directUsers := core.Users(core.UserWhere.ID.IN(directUserIDs)).AllP(ctx, db)
	secondDegreeUsers := core.Users(core.UserWhere.ID.IN(secondDegreeUserIDs)).AllP(ctx, db)

	whitelistedConnections := lo.Map(
		core.WhitelistedConnections(
			core.WhitelistedConnectionWhere.WhoID.EQ(userID),
			core.WhitelistedConnectionWhere.ConnectionID.IsNull(),
			qm.Load(core.WhitelistedConnectionRels.AllowsWho),
		).AllP(ctx, db),
		func(conn *core.WhitelistedConnection, idx int) *core.User {
			return conn.R.AllowsWho
		})

	connectionRequestsFromMediation, err := core.UserConnectionMediationRequests(
		core.UserConnectionMediationRequestWhere.TargetUserID.EQ(userID),
		core.UserConnectionMediationRequestWhere.TargetDecision.IsNull(),
		qm.Load(core.UserConnectionMediationRequestRels.WhoUser),
		qm.Load(qm.Rels(
			core.UserConnectionMediationRequestRels.MediationUserConnectionMediators,
			core.UserConnectionMediatorRels.User,
		)),
		qm.Load(core.UserConnectionMediationRequestRels.MediationUserConnectionMediators,
			core.UserConnectionMediatorWhere.Decision.EQ(core.ConnectionMediationDecisionSigned),
		),
	).All(ctx, db)

	if err != nil {
		return mo.Err[*ControlsPage](err)
	}

	connectionRequests := []*ConnectionRequest{}

	for _, req := range connectionRequestsFromMediation {
		if len(req.R.MediationUserConnectionMediators) == 0 {
			continue
		}

		connectionRequests = append(connectionRequests, &ConnectionRequest{
			Requester: req.R.WhoUser,
			Request:   req,
			Mediations: lo.Map(req.R.MediationUserConnectionMediators, func(m *core.UserConnectionMediator, idx int) *MediationResult {
				return &MediationResult{
					Mediator:  m.R.User,
					Mediation: m,
				}
			}),
		})
	}

	mediationRequestsDB, err := core.UserConnectionMediationRequests(
		core.UserConnectionMediationRequestWhere.WhoUserID.IN(directUserIDs),
		core.UserConnectionMediationRequestWhere.TargetUserID.IN(directUserIDs),
		core.UserConnectionMediationRequestWhere.TargetDecision.IsNull(),
		qm.Load(
			core.UserConnectionMediationRequestRels.WhoUser,
		),
		qm.Load(
			core.UserConnectionMediationRequestRels.TargetUser,
		),
		qm.Load(
			core.UserConnectionMediationRequestRels.MediationUserConnectionMediators,
			core.UserConnectionMediatorWhere.UserID.EQ(userID),
		),
	).All(ctx, db)

	if err != nil {
		return mo.Err[*ControlsPage](err)
	}

	mediationRequestsDB = lo.Filter(mediationRequestsDB, func(req *core.UserConnectionMediationRequest, idx int) bool {
		return len(req.R.MediationUserConnectionMediators) == 0
	})

	mediationRequests := lo.Map(mediationRequestsDB, func(req *core.UserConnectionMediationRequest, idx int) *MediationRequest {
		return &MediationRequest{
			Requester: req.R.WhoUser,
			Target:    req.R.TargetUser,
			Request:   req,
		}
	})

	rawDrafts, err := core.Posts(
		core.PostWhere.UserID.EQ(userID),
		core.PostWhere.PublishedAt.IsNull(),
		qm.OrderBy(fmt.Sprintf("%s DESC", core.PostColumns.UpdatedAt)),
	).All(ctx, db)

	if err != nil {
		return mo.Err[*ControlsPage](err)
	}

	drafts := lo.Map(rawDrafts, func(d *core.Post, idx int) *Draft {
		return &Draft{
			PostID:        d.ID,
			Subject:       d.Subject.String,
			LastUpdatedAt: d.UpdatedAt.Time,
		}
	})

	controlsPage := &ControlsPage{
		BasePage:                getBasePage(ctx, "Controls", userData),
		DirectConnections:       directUsers,
		SecondDegreeConnections: secondDegreeUsers,
		WhitelistedConnections:  whitelistedConnections,
		ConnectionRequests:      connectionRequests,
		MediationRequests:       mediationRequests,
		Drafts:                  drafts,
	}

	return mo.Ok(controlsPage)
}
