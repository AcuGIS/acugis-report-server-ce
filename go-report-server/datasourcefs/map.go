package datasourcefs

import (
	"strings"

	"github.com/acugis/acugis-report-server-go/internal/datasources"
)

// secretOptionKeys are never copied into Record.Options.
var secretOptionKeys = map[string]struct{}{
	"credentialsresourceuuid": {},
	"privatekeypem":           {},
	"private_key":             {},
	"password":                {},
	"token":                   {},
	"authorization":           {},
	"httpheadervalues":        {},
	"bearer":                  {},
}

// MapDatasource copies one jasper_datasource row. privateKey and credentials
// are the bytes already read by the content-resource store; either may be nil.
func MapDatasource(ds datasources.Datasource, privateKey, credentials []byte) Record {
	opts := map[string]string{}
	for k, v := range ds.Options {
		if _, secret := secretOptionKeys[strings.ToLower(strings.TrimSpace(k))]; secret {
			continue
		}
		opts[k] = v
	}
	return Record{
		ID:                      ds.ID,
		Registry:                RegistryDatasource,
		Name:                    strings.TrimSpace(ds.Name),
		DBType:                  datasources.NormalizeDBType(ds.DBType),
		Host:                    strings.TrimSpace(ds.Host),
		Port:                    ds.Port,
		Database:                strings.TrimSpace(ds.DBName),
		Username:                strings.TrimSpace(ds.Username),
		Schema:                  strings.TrimSpace(ds.SchemaName),
		AuthType:                datasources.NormalizeAuthType(ds.AuthType),
		Password:                ds.Password,
		PrivateKeyPEM:           append([]byte(nil), privateKey...),
		CredentialsJSON:         append([]byte(nil), credentials...),
		PrivateKeyResourceUUID:  strings.TrimSpace(ds.PrivateKeyResourceUUID),
		CredentialsResourceUUID: ds.CredentialsResourceUUID(),
		Options:                 opts,
	}
}

// MapGeoServer copies one jasper_geoserver row.
func MapGeoServer(id int64, name, rawURL, username, password string) Record {
	return Record{
		ID:       id,
		Registry: RegistryGeoServer,
		Name:     strings.TrimSpace(name),
		DBType:   "jasper_geoserver",
		Host:     strings.TrimRight(strings.TrimSpace(rawURL), "/"),
		Username: strings.TrimSpace(username),
		Password: password,
	}
}

// MapQGIS copies one jasper_qgis row. project is the existing MAP project field.
func MapQGIS(id int64, name, rawURL, username, password, project string) Record {
	opts := map[string]string{}
	if p := strings.TrimSpace(project); p != "" {
		opts["project"] = p
	}
	return Record{
		ID:       id,
		Registry: RegistryQGIS,
		Name:     strings.TrimSpace(name),
		DBType:   "jasper_qgis",
		Host:     strings.TrimRight(strings.TrimSpace(rawURL), "/"),
		Username: strings.TrimSpace(username),
		Password: password,
		Options:  opts,
	}
}

// MapArcGIS copies one jasper_arcgis row. This is a map/collection connection,
// not a JDBC datasource.
func MapArcGIS(id int64, name, rawURL, username, password string) Record {
	return Record{
		ID:       id,
		Registry: RegistryArcGIS,
		Name:     strings.TrimSpace(name),
		DBType:   "jasper_arcgis",
		Host:     strings.TrimRight(strings.TrimSpace(rawURL), "/"),
		Username: strings.TrimSpace(username),
		Password: password,
	}
}
