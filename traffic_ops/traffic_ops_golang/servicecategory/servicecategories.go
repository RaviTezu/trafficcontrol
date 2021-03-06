package servicecategory

/*
 * Licensed to the Apache Software Foundation (ASF) under one
 * or more contributor license agreements.  See the NOTICE file
 * distributed with this work for additional information
 * regarding copyright ownership.  The ASF licenses this file
 * to you under the Apache License, Version 2.0 (the
 * "License"); you may not use this file except in compliance
 * with the License.  You may obtain a copy of the License at
 *
 *   http://www.apache.org/licenses/LICENSE-2.0
 *
 * Unless required by applicable law or agreed to in writing,
 * software distributed under the License is distributed on an
 * "AS IS" BASIS, WITHOUT WARRANTIES OR CONDITIONS OF ANY
 * KIND, either express or implied.  See the License for the
 * specific language governing permissions and limitations
 * under the License.
 */

import (
	"errors"
	"net/http"
	"strconv"
	"time"

	"github.com/apache/trafficcontrol/lib/go-tc"
	"github.com/apache/trafficcontrol/lib/go-tc/tovalidate"
	"github.com/apache/trafficcontrol/lib/go-util"
	"github.com/apache/trafficcontrol/traffic_ops/traffic_ops_golang/api"
	"github.com/apache/trafficcontrol/traffic_ops/traffic_ops_golang/dbhelpers"
	"github.com/apache/trafficcontrol/traffic_ops/traffic_ops_golang/tenant"
	"github.com/go-ozzo/ozzo-validation"
)

type TOServiceCategory struct {
	api.APIInfoImpl `json:"-"`
	tc.ServiceCategory
}

func (v *TOServiceCategory) GetLastUpdated() (*time.Time, bool, error) {
	found := false
	lastUpdated := time.Time{}
	rows, err := v.APIInfo().Tx.Query(`select last_updated from service_category where name=$1`, v.Name)
	if err != nil {
		return nil, found, errors.New("querying last_updated: " + err.Error())
	}
	defer rows.Close()
	if !rows.Next() {
		return nil, found, errors.New("no resource found with this id")
	}
	found = true
	if err := rows.Scan(&lastUpdated); err != nil {
		return nil, found, errors.New("scanning last_updated: " + err.Error())
	}
	return &lastUpdated, found, nil
}

func (v *TOServiceCategory) SetLastUpdated(t tc.TimeNoMod) { v.LastUpdated = t }
func (v *TOServiceCategory) InsertQuery() string           { return insertQuery() }
func (v *TOServiceCategory) NewReadObj() interface{}       { return &tc.ServiceCategory{} }
func (v *TOServiceCategory) SelectQuery() string           { return selectQuery() }
func (v *TOServiceCategory) UpdateQuery() string           { return updateQuery() }
func (v *TOServiceCategory) DeleteQuery() string           { return deleteQuery() }

func (serviceCategory TOServiceCategory) GetAuditName() string {
	if serviceCategory.Name != "" {
		return serviceCategory.Name
	}
	if serviceCategory.TenantID != 0 {
		return strconv.Itoa(serviceCategory.TenantID)
	}
	return "unknown"
}

func (serviceCategory TOServiceCategory) GetKeyFieldsInfo() []api.KeyFieldInfo {
	return []api.KeyFieldInfo{{"name", api.GetStringKey}}
}

//Implementation of the Identifier, Validator interface functions
func (serviceCategory TOServiceCategory) GetKeys() (map[string]interface{}, bool) {
	if serviceCategory.Name == "" {
		return map[string]interface{}{"name": ""}, false
	}
	return map[string]interface{}{"name": serviceCategory.Name}, true
}

func (serviceCategory *TOServiceCategory) SetKeys(keys map[string]interface{}) {
	n, _ := keys["name"].(string)
	serviceCategory.Name = n
}

func (serviceCategory TOServiceCategory) GetType() string {
	return "serviceCategory"
}

func (serviceCategory *TOServiceCategory) ParamColumns() map[string]dbhelpers.WhereColumnInfo {
	return map[string]dbhelpers.WhereColumnInfo{
		"name":       dbhelpers.WhereColumnInfo{"sc.name", nil},
		"tenantId":   dbhelpers.WhereColumnInfo{"sc.tenant_id", api.IsInt},
		"tenantName": dbhelpers.WhereColumnInfo{"sc.tenant", nil},
	}
}

func (serviceCategory *TOServiceCategory) SelectMaxLastUpdatedQuery(where, orderBy, pagination, tableName string) string {
	return `SELECT max(t) from (
		SELECT max(last_updated) as t from service_category sc ` + where + orderBy + pagination +
		` UNION ALL
	select max(last_updated) as t from last_deleted l where l.table_name='service_category') as res`
}

func (serviceCategory TOServiceCategory) Validate() error {
	errs := validation.Errors{
		"name":     validation.Validate(serviceCategory.Name, validation.NotNil, validation.Required),
		"tenantId": validation.Validate(serviceCategory.TenantID, validation.Min(1)),
	}
	return util.JoinErrs(tovalidate.ToErrors(errs))
}

func (serviceCategory *TOServiceCategory) Create() (error, error, int) {
	return api.GenericCreate(serviceCategory)
}

func (serviceCategory *TOServiceCategory) Read(h http.Header, useIMS bool) ([]interface{}, error, error, int, *time.Time) {
	tenantIDs, err := tenant.GetUserTenantIDListTx(serviceCategory.APIInfo().Tx.Tx, serviceCategory.APIInfo().User.TenantID)
	if err != nil {
		return nil, nil, errors.New("getting tenant list for user: " + err.Error()), http.StatusInternalServerError, nil
	}

	api.DefaultSort(serviceCategory.APIInfo(), "name")
	serviceCategories, userErr, sysErr, errCode, maxTime := api.GenericRead(h, serviceCategory, useIMS)
	if userErr != nil || sysErr != nil {
		return nil, userErr, sysErr, errCode, nil
	}

	filteredServiceCategories := []interface{}{}
	for _, sc := range serviceCategories {
		scToCheck := sc.(*tc.ServiceCategory)
		if checkTenancy(scToCheck, tenantIDs) {
			filteredServiceCategories = append(filteredServiceCategories, scToCheck)
		}
	}

	dsIdParam := serviceCategory.APIInfo().Params["dsId"]

	if dsIdParam != "" {
		dsId, err := strconv.Atoi(dsIdParam)
		if err != nil {
			return nil, errors.New("dsId query param must be an int"), nil, http.StatusBadRequest, nil
		}
		dsTenantId, _, err := tenant.GetDSTenantIDByIDTx(serviceCategory.APIInfo().Tx.Tx, dsId)

		dsAllowedTenants, err := tenant.GetUserTenantIDListTx(serviceCategory.APIInfo().Tx.Tx, *dsTenantId)
		if err != nil {
			return nil, nil, errors.New("getting tenant list for user: " + err.Error()), http.StatusInternalServerError, nil
		}

		filteredSCByDS := []interface{}{}
		for _, sc := range filteredServiceCategories {
			scToCheck := sc.(*tc.ServiceCategory)
			if checkTenancy(scToCheck, dsAllowedTenants) {
				filteredSCByDS = append(filteredSCByDS, scToCheck)
			}
		}

		return filteredSCByDS, nil, nil, errCode, maxTime
	}

	return filteredServiceCategories, nil, nil, errCode, maxTime
}

func checkTenancy(category *tc.ServiceCategory, tenantIDs []int) bool {
	for _, tenantID := range tenantIDs {
		if tenantID == category.TenantID {
			return true
		}
	}
	return false
}

func (serviceCategory *TOServiceCategory) Update(h http.Header) (error, error, int) {
	return api.GenericUpdate(h, serviceCategory)
}
func (serviceCategory *TOServiceCategory) Delete() (error, error, int) {
	return api.GenericDelete(serviceCategory)
}

func insertQuery() string {
	return `INSERT INTO service_category (name, tenant_id) VALUES (:name, :tenant_id) RETURNING name, last_updated`
}

func selectQuery() string {
	return `SELECT
sc.tenant_id,
t.name as tenant,
sc.last_updated,
sc.name
FROM service_category as sc
LEFT JOIN tenant t ON sc.tenant_id = t.id`
}

func updateQuery() string {
	return `UPDATE
service_category SET
name=:name,
tenant_id=:tenant_id
WHERE name=:name RETURNING last_updated`
}

func deleteQuery() string {
	return `DELETE FROM service_category WHERE name=:name`
}
