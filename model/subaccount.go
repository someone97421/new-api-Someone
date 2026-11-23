package model

import (
	"errors"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/relaykit/dto"
	"gorm.io/gorm"
	gormlogger "gorm.io/gorm/logger"
)

var (
	ErrSubaccountForbidden    = errors.New("subaccount operation forbidden")
	ErrSubaccountReauth       = errors.New("subaccount operation requires recent login")
	ErrInvalidSubaccountInput = errors.New("invalid subaccount input")
)

const subaccountLoginMaxAge = 5 * time.Minute

type SubaccountSummary struct {
	Id           int    `json:"id"`
	Username     string `json:"username"`
	Status       int    `json:"status"`
	UsedQuota    int    `json:"used_quota"`
	RequestCount int    `json:"request_count"`
}

func ListSubaccounts(parentID int) ([]SubaccountSummary, error) {
	if err := requireSubaccountParent(DB, parentID); err != nil {
		return nil, err
	}
	children := make([]SubaccountSummary, 0)
	err := DB.Model(&User{}).Select("id", "username", "status", "used_quota", "request_count").
		Where("parent_id = ?", parentID).Order("id DESC").Find(&children).Error
	return children, err
}

func requireSubaccountParent(tx *gorm.DB, parentID int) error {
	if parentID <= 0 {
		return ErrSubaccountForbidden
	}
	var parent User
	err := tx.Select("id").Where("id = ? AND parent_id = ? AND status = ?", parentID, 0, common.UserStatusEnabled).First(&parent).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return ErrSubaccountForbidden
	}
	return err
}

func authorizeSubaccountMutation(tx *gorm.DB, identity AuthSessionIdentity) error {
	if _, isPAT := ParseAccessTokenSessionID(identity.SessionID); isPAT {
		return ErrSubaccountForbidden
	}
	if err := ValidateAuthSessionWithTx(tx, identity); err != nil {
		if errors.Is(err, ErrUserSessionInactive) || errors.Is(err, gorm.ErrRecordNotFound) {
			return ErrSubaccountForbidden
		}
		return err
	}
	if err := requireSubaccountParent(tx, identity.UserID); err != nil {
		return err
	}
	var session UserSession
	if err := tx.Select("created_at").Where("sid = ? AND user_id = ?", identity.SessionID, identity.UserID).Take(&session).Error; err != nil {
		return err
	}
	now := time.Now().Unix()
	if session.CreatedAt > now || session.CreatedAt <= now-int64(subaccountLoginMaxAge/time.Second) {
		return ErrSubaccountReauth
	}
	return nil
}

func CreateSubaccount(identity AuthSessionIdentity, username, password string) (*SubaccountSummary, error) {
	username = strings.TrimSpace(username)
	if username == "" || !utf8.ValidString(username) || utf8.RuneCountInString(username) > 20 {
		return nil, ErrInvalidSubaccountInput
	}
	child := User{
		ParentId:    identity.UserID,
		Username:    username,
		DisplayName: username,
		Role:        common.RoleCommonUser,
		Status:      common.UserStatusEnabled,
	}
	child.SetSetting(dto.UserSetting{SidebarModules: generateDefaultSidebarConfigForRole(child.Role)})
	err := DB.Transaction(func(tx *gorm.DB) error {
		if err := authorizeSubaccountMutation(tx, identity); err != nil {
			return err
		}
		var err error
		child.Password, err = common.HashAccountPassword(password)
		if err != nil {
			return err
		}
		child.AffCode, err = common.GenerateRandomCharsKey(20)
		if err != nil {
			return err
		}
		return tx.Session(&gorm.Session{Logger: tx.Logger.LogMode(gormlogger.Silent)}).Create(&child).Error
	})
	if err != nil {
		return nil, err
	}
	if err := populateUserCache(child); err != nil {
		common.SysLog(common.LogText("failed to populate cache for new subaccount %d", child.Id))
	}
	return &SubaccountSummary{Id: child.Id, Username: child.Username, Status: child.Status}, nil
}

func DisableSubaccount(identity AuthSessionIdentity, childID int) error {
	if childID <= 0 || childID == identity.UserID {
		return ErrSubaccountForbidden
	}
	var changed bool
	err := DB.Transaction(func(tx *gorm.DB) error {
		if err := authorizeSubaccountMutation(tx, identity); err != nil {
			return err
		}
		var child User
		const ownedChild = "id = ? AND parent_id = ? AND role = ?"
		err := lockForUpdate(tx).Select("id", "status").Where(ownedChild, childID, identity.UserID, common.RoleCommonUser).First(&child).Error
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return ErrSubaccountForbidden
		}
		if err != nil || child.Status == common.UserStatusDisabled {
			return err
		}
		if _, err := IncrementUserAuthVersionWithTx(tx, childID); err != nil {
			return err
		}
		result := tx.Model(&User{}).Where(ownedChild, childID, identity.UserID, common.RoleCommonUser).Update("status", common.UserStatusDisabled)
		if result.Error != nil {
			return result.Error
		}
		if result.RowsAffected != 1 {
			return ErrSubaccountForbidden
		}
		changed = true
		return nil
	})
	if err != nil || !changed {
		return err
	}
	if err := PublishUserAuthCache(childID); err != nil {
		common.SysLog(common.LogText("failed to publish disabled subaccount %d authentication cache", childID))
	}
	if _, err := RevokeAllUserSessions(childID, "subaccount_disabled"); err != nil {
		common.SysLog(common.LogText("failed to revoke sessions for disabled subaccount %d", childID))
	}
	return nil
}

type AdminUserRow struct {
	*User
	ParentID        int    `json:"parent_id,omitempty"`
	ParentUsername  string `json:"parent_username,omitempty"`
	SubaccountCount int    `json:"subaccount_count,omitempty"`
}

// AdminUserRows resolves relationships outside the current page and filters.
func AdminUserRows(users []*User) ([]AdminUserRow, error) {
	rows := make([]AdminUserRow, len(users))
	parentIDs := make([]int, 0, len(users))
	rootIDs := make([]int, 0, len(users))
	for i, user := range users {
		rows[i] = AdminUserRow{User: user, ParentID: user.ParentId}
		if user.ParentId > 0 {
			parentIDs = append(parentIDs, user.ParentId)
		} else {
			rootIDs = append(rootIDs, user.Id)
		}
	}
	parentNames := make(map[int]string, len(parentIDs))
	if len(parentIDs) > 0 {
		var parents []struct {
			Id       int
			Username string
		}
		if err := DB.Unscoped().Model(&User{}).Select("id", "username").Where("id IN ?", parentIDs).Find(&parents).Error; err != nil {
			return nil, err
		}
		for _, parent := range parents {
			parentNames[parent.Id] = parent.Username
		}
	}
	counts := make(map[int]int, len(rootIDs))
	if len(rootIDs) > 0 {
		var children []struct {
			ParentID int
			Total    int
		}
		if err := DB.Unscoped().Model(&User{}).Select("parent_id, COUNT(*) AS total").Where("parent_id IN ?", rootIDs).Group("parent_id").Find(&children).Error; err != nil {
			return nil, err
		}
		for _, child := range children {
			counts[child.ParentID] = child.Total
		}
	}
	for i := range rows {
		rows[i].ParentUsername = parentNames[rows[i].ParentID]
		rows[i].SubaccountCount = counts[rows[i].Id]
	}
	return rows, nil
}
