package store

import (
	"context"
	"errors"
	"fmt"
	"time"

	"gorm.io/gorm"
)

// AccountRole 区分两个账户表。
type AccountRole string

// 账户角色，每个表一个。
const (
	RoleAdmin    AccountRole = "admin"
	RoleCustomer AccountRole = "customer"
)

// Account 是对管理员或客户记录的统一视图。
type Account struct {
	ID           string
	Username     string
	PasswordHash string
	Role         AccountRole
	Disabled     bool
	UsedBytes    int64
	QuotaBytes   int64
	// RoleGroupID 仅对客户有意义，指向其所属角色组。
	RoleGroupID *string
	CreatedAt   time.Time
	UpdatedAt   time.Time
}

// table 返回支撑给定角色的 gorm 模型。
func tableFor(role AccountRole) any {
	if role == RoleAdmin {
		return &Admin{}
	}
	return &Customer{}
}

// CreateAdmin 插入一个管理员账户。
func (r *Repository) CreateAdmin(ctx context.Context, admin *Admin) error {
	if err := r.db.WithContext(ctx).Create(admin).Error; err != nil {
		return fmt.Errorf("store: create admin: %w", err)
	}
	return nil
}

// CreateCustomer 插入一个客户账户。
func (r *Repository) CreateCustomer(ctx context.Context, customer *Customer) error {
	if err := r.db.WithContext(ctx).Create(customer).Error; err != nil {
		return fmt.Errorf("store: create customer: %w", err)
	}
	return nil
}

// GetAccountByID 从给定角色的表中返回具有 id 的账户，或
// ErrNotFound。
func (r *Repository) GetAccountByID(ctx context.Context, role AccountRole, id string) (*Account, error) {
	if role == RoleAdmin {
		var admin Admin
		if err := r.first(ctx, &admin, "id = ?", id); err != nil {
			return nil, fmt.Errorf("store: admin %q: %w", id, err)
		}
		return accountFromAdmin(&admin), nil
	}
	var customer Customer
	if err := r.first(ctx, &customer, "id = ?", id); err != nil {
		return nil, fmt.Errorf("store: customer %q: %w", id, err)
	}
	return accountFromCustomer(&customer), nil
}

// GetAccountByUsername 从给定角色的表中返回具有 username 的账户，
// 或 ErrNotFound。用户名在每个表中唯一，因此管理员和客户可以
// 共享同一个用户名。
func (r *Repository) GetAccountByUsername(ctx context.Context, role AccountRole, username string) (*Account, error) {
	if role == RoleAdmin {
		var admin Admin
		if err := r.first(ctx, &admin, "username = ?", username); err != nil {
			return nil, fmt.Errorf("store: admin %q: %w", username, err)
		}
		return accountFromAdmin(&admin), nil
	}
	var customer Customer
	if err := r.first(ctx, &customer, "username = ?", username); err != nil {
		return nil, fmt.Errorf("store: customer %q: %w", username, err)
	}
	return accountFromCustomer(&customer), nil
}

// first 将第一个匹配的行加载到 dest 中，并把 gorm 的未找到错误映射
// 为 ErrNotFound。
func (r *Repository) first(ctx context.Context, dest any, query string, args ...any) error {
	conditions := append([]any{query}, args...)
	err := r.db.WithContext(ctx).First(dest, conditions...).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return ErrNotFound
	}
	return err
}

// ListCustomers 返回按创建时间排序的所有客户账户。
func (r *Repository) ListCustomers(ctx context.Context) ([]Account, error) {
	var customers []Customer
	if err := r.db.WithContext(ctx).Order("created_at ASC").Find(&customers).Error; err != nil {
		return nil, fmt.Errorf("store: list customers: %w", err)
	}
	accounts := make([]Account, 0, len(customers))
	for i := range customers {
		accounts = append(accounts, *accountFromCustomer(&customers[i]))
	}
	return accounts, nil
}

// ListAdmins 返回按创建时间排序的所有管理员账户。
func (r *Repository) ListAdmins(ctx context.Context) ([]Account, error) {
	var admins []Admin
	if err := r.db.WithContext(ctx).Order("created_at ASC").Find(&admins).Error; err != nil {
		return nil, fmt.Errorf("store: list admins: %w", err)
	}
	accounts := make([]Account, 0, len(admins))
	for i := range admins {
		accounts = append(accounts, *accountFromAdmin(&admins[i]))
	}
	return accounts, nil
}

// UsernamesByIDs 返回账户 id 到用户名的映射，用于批量填充图片所有者。不存在
// 的 id 不会出现在结果中。
func (r *Repository) UsernamesByIDs(ctx context.Context, ids []string) (map[string]string, error) {
	result := make(map[string]string, len(ids))
	if len(ids) == 0 {
		return result, nil
	}
	type row struct {
		ID       string
		Username string
	}
	var rows []row
	if err := r.db.WithContext(ctx).Model(&Customer{}).
		Select("id, username").
		Where("id IN ?", ids).
		Scan(&rows).Error; err != nil {
		return nil, fmt.Errorf("store: usernames by ids: %w", err)
	}
	for _, row := range rows {
		result[row.ID] = row.Username
	}
	return result, nil
}

// CountAdmins 返回管理员账户的数量。
func (r *Repository) CountAdmins(ctx context.Context) (int64, error) {
	var count int64
	if err := r.db.WithContext(ctx).Model(&Admin{}).Count(&count).Error; err != nil {
		return 0, fmt.Errorf("store: count admins: %w", err)
	}
	return count, nil
}

// CountEnabledAdmins 返回已启用管理员账户的数量。
func (r *Repository) CountEnabledAdmins(ctx context.Context) (int64, error) {
	var count int64
	if err := r.db.WithContext(ctx).Model(&Admin{}).
		Where("disabled = ?", false).
		Count(&count).Error; err != nil {
		return 0, fmt.Errorf("store: count enabled admins: %w", err)
	}
	return count, nil
}

// CountCustomers 返回客户账户的数量。
func (r *Repository) CountCustomers(ctx context.Context) (int64, error) {
	var count int64
	if err := r.db.WithContext(ctx).Model(&Customer{}).Count(&count).Error; err != nil {
		return 0, fmt.Errorf("store: count customers: %w", err)
	}
	return count, nil
}

// UpdateCustomer 更新客户的禁用标志与角色组并返回结果。
func (r *Repository) UpdateCustomer(ctx context.Context, id string, update UserUpdate) (*Account, error) {
	fields := map[string]any{}
	if update.Disabled != nil {
		fields["disabled"] = *update.Disabled
	}
	if update.RoleGroupID != nil {
		if *update.RoleGroupID == "" {
			fields["role_group_id"] = nil
		} else {
			fields["role_group_id"] = *update.RoleGroupID
		}
	}
	if update.QuotaBytes != nil {
		fields["quota_bytes"] = *update.QuotaBytes
	}
	if len(fields) > 0 {
		if err := r.db.WithContext(ctx).Model(&Customer{}).Where("id = ?", id).Updates(fields).Error; err != nil {
			return nil, fmt.Errorf("store: update customer %q: %w", id, err)
		}
	}
	return r.GetAccountByID(ctx, RoleCustomer, id)
}

// UpdateAdmin 更新管理员的禁用标志并返回结果。
func (r *Repository) UpdateAdmin(ctx context.Context, id string, update UserUpdate) (*Account, error) {
	if err := r.db.WithContext(ctx).Model(&Admin{}).Where("id = ?", id).Updates(update).Error; err != nil {
		return nil, fmt.Errorf("store: update admin %q: %w", id, err)
	}
	return r.GetAccountByID(ctx, RoleAdmin, id)
}

// ReserveQuota 在不超过配额的情况下，原子性地增加客户的已用字节数。
// 当会超出配额时返回 false。
func (r *Repository) ReserveQuota(ctx context.Context, customerID string, amount int64) (bool, error) {
	result := r.db.WithContext(ctx).Model(&Customer{}).
		Where("id = ? AND (quota_bytes = 0 OR used_bytes + ? <= quota_bytes)", customerID, amount).
		UpdateColumn("used_bytes", gorm.Expr("used_bytes + ?", amount))
	if result.Error != nil {
		return false, fmt.Errorf("store: reserve quota for %q: %w", customerID, result.Error)
	}
	return result.RowsAffected == 1, nil
}

// ReleaseQuota 减少客户的已用字节数。
func (r *Repository) ReleaseQuota(ctx context.Context, customerID string, amount int64) error {
	if err := r.db.WithContext(ctx).Model(&Customer{}).
		Where("id = ? AND used_bytes >= ?", customerID, amount).
		UpdateColumn("used_bytes", gorm.Expr("used_bytes - ?", amount)).Error; err != nil {
		return fmt.Errorf("store: release quota for %q: %w", customerID, err)
	}
	return nil
}

// DeleteAccount 从支撑其角色的表中移除一个账户。
func (r *Repository) DeleteAccount(ctx context.Context, role AccountRole, id string) error {
	result := r.db.WithContext(ctx).Delete(tableFor(role), "id = ?", id)
	if result.Error != nil {
		return fmt.Errorf("store: delete account %q: %w", id, result.Error)
	}
	if result.RowsAffected == 0 {
		return fmt.Errorf("store: account %q: %w", id, ErrNotFound)
	}
	return nil
}

func accountFromAdmin(admin *Admin) *Account {
	return &Account{
		ID:           admin.ID,
		Username:     admin.Username,
		PasswordHash: admin.PasswordHash,
		Role:         RoleAdmin,
		Disabled:     admin.Disabled,
		CreatedAt:    admin.CreatedAt,
		UpdatedAt:    admin.UpdatedAt,
	}
}

func accountFromCustomer(customer *Customer) *Account {
	return &Account{
		ID:           customer.ID,
		Username:     customer.Username,
		PasswordHash: customer.PasswordHash,
		Role:         RoleCustomer,
		Disabled:     customer.Disabled,
		UsedBytes:    customer.UsedBytes,
		QuotaBytes:   customer.QuotaBytes,
		RoleGroupID:  customer.RoleGroupID,
		CreatedAt:    customer.CreatedAt,
		UpdatedAt:    customer.UpdatedAt,
	}
}
