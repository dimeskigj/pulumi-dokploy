package dokploy

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"testing"

	"github.com/pulumi/pulumi-go-provider/infer"
	"github.com/stretchr/testify/require"
)

func TestMountTargetDispatchDeploysAndPollsEveryTarget(t *testing.T) {
	cases := []struct {
		name, statusPath, deployPath, queryKey, id, statusField, deployBody string
		args                                                                MountArgs
	}{
		{"application", "/api/application.one", "/api/application.redeploy", "applicationId", "a1", "applicationStatus", `{"applicationId":"a1"}`, MountArgs{ApplicationID: stringPtr("a1")}},
		{"compose", "/api/compose.one", "/api/compose.redeploy", "composeId", "c1", "composeStatus", `{"composeId":"c1"}`, MountArgs{ComposeID: stringPtr("c1")}},
		{"postgres", "/api/postgres.one", "/api/postgres.deploy", "postgresId", "p1", "applicationStatus", `{"postgresId":"p1"}`, MountArgs{PostgresID: stringPtr("p1")}},
		{"mysql", "/api/mysql.one", "/api/mysql.deploy", "mysqlId", "m1", "applicationStatus", `{"mysqlId":"m1"}`, MountArgs{MySQLID: stringPtr("m1")}},
		{"mariadb", "/api/mariadb.one", "/api/mariadb.deploy", "mariadbId", "md1", "applicationStatus", `{"mariadbId":"md1"}`, MountArgs{MariaDBID: stringPtr("md1")}},
		{"redis", "/api/redis.one", "/api/redis.deploy", "redisId", "r1", "applicationStatus", `{"redisId":"r1"}`, MountArgs{RedisID: stringPtr("r1")}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			s := newScriptedServer(t,
				expectGET(tc.statusPath, map[string][]string{tc.queryKey: {tc.id}}, http.StatusOK, `{"`+tc.queryKey+`":"`+tc.id+`","`+tc.statusField+`":"done"}`),
				scriptedRequest{Method: http.MethodPost, Path: tc.deployPath, Body: rawJSON(tc.deployBody), Status: http.StatusOK, Response: []byte(`{}`)},
				expectGET(tc.statusPath, map[string][]string{tc.queryKey: {tc.id}}, http.StatusOK, `{"`+tc.queryKey+`":"`+tc.id+`","`+tc.statusField+`":"done"}`),
			)
			target, err := mountTargetFor(tc.args)
			require.NoError(t, err)
			exists, err := deployMountTarget(t.Context(), s.API(), target)
			require.NoError(t, err)
			require.True(t, exists)
		})
	}
}

func TestMountCreateBodyMatrix(t *testing.T) {
	cases := []struct {
		name, body, response string
		args                 MountArgs
	}{
		{"bind", `{"content":null,"filePath":null,"hostPath":"/host","mountPath":"/data","serviceId":"a1","serviceType":"application","type":"bind","volumeName":null}`, `{"mountId":"m1"}`, MountArgs{Type: "bind", MountPath: "/data", HostPath: stringPtr("/host"), ApplicationID: stringPtr("a1")}},
		{"volume", `{"content":null,"filePath":null,"hostPath":null,"mountPath":"/data","serviceId":"a1","serviceType":"application","type":"volume","volumeName":"vol"}`, `{"mountId":"m2"}`, MountArgs{Type: "volume", MountPath: "/data", VolumeName: stringPtr("vol"), ApplicationID: stringPtr("a1")}},
		{"file", `{"content":"hello","filePath":"/etc/config","hostPath":null,"mountPath":"/data","serviceId":"a1","serviceType":"application","type":"file","volumeName":null}`, `{"mountId":"m3"}`, MountArgs{Type: "file", MountPath: "/data", FilePath: stringPtr("/etc/config"), Content: stringPtr("hello"), ApplicationID: stringPtr("a1")}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			s := newScriptedServer(t,
				scriptedRequest{Method: http.MethodPost, Path: "/api/mounts.create", Body: rawJSON(tc.body), Status: http.StatusOK, Response: []byte(tc.response)},
				expectGET("/api/mounts.one", map[string][]string{"mountId": {tc.response[12:14]}}, http.StatusOK, `{"mountId":"`+tc.response[12:14]+`","mountPath":"`+tc.args.MountPath+`","type":"`+tc.args.Type+`","serviceType":"application","applicationId":"a1"}`),
				expectGET("/api/application.one", map[string][]string{"applicationId": {"a1"}}, http.StatusOK, `{"applicationId":"a1","applicationStatus":"done"}`),
				expectPOST("/api/application.redeploy", `{"applicationId":"a1"}`, `{}`),
				expectGET("/api/application.one", map[string][]string{"applicationId": {"a1"}}, http.StatusOK, `{"applicationId":"a1","applicationStatus":"done"}`),
			)
			got, err := (Mount{client: fixedClient(s.API())}).Create(t.Context(), infer.CreateRequest[MountArgs]{Inputs: tc.args})
			require.NoError(t, err)
			require.Equal(t, tc.response[12:14], got.ID)
		})
	}
}

func TestMountUpdateBodyClearsOptionalValuesWithExplicitNulls(t *testing.T) {
	s := newScriptedServer(t,
		expectPOST("/api/mounts.update", `{"applicationId":"a1","composeId":null,"content":null,"filePath":null,"hostPath":null,"mariadbId":null,"mountId":"m1","mountPath":"/data","mysqlId":null,"postgresId":null,"redisId":null,"serviceType":"application","type":"bind","volumeName":null}`, `{}`),
		expectGET("/api/mounts.one", map[string][]string{"mountId": {"m1"}}, http.StatusOK, `{"mountId":"m1","mountPath":"/data","type":"bind","serviceType":"application","applicationId":"a1"}`),
		expectGET("/api/application.one", map[string][]string{"applicationId": {"a1"}}, http.StatusOK, `{"applicationId":"a1","applicationStatus":"done"}`),
		expectPOST("/api/application.redeploy", `{"applicationId":"a1"}`, `{}`),
		expectGET("/api/application.one", map[string][]string{"applicationId": {"a1"}}, http.StatusOK, `{"applicationId":"a1","applicationStatus":"done"}`),
	)
	got, err := (Mount{client: fixedClient(s.API())}).Update(t.Context(), infer.UpdateRequest[MountArgs, MountState]{ID: "m1", Inputs: MountArgs{Type: "bind", MountPath: "/data", ApplicationID: stringPtr("a1")}})
	require.NoError(t, err)
	require.Nil(t, got.Output.HostPath)
}

func TestMountUpdateBodyAndRedeployMatrix(t *testing.T) {
	targets := mountLifecycleTargetCases()
	mounts := []struct {
		name, typ, mountFields, updateFields string
		args                                 MountArgs
	}{
		{"bind", "bind", `"hostPath":"/host"`, `"hostPath":"/new-host"`, MountArgs{Type: "bind", MountPath: "/data", HostPath: stringPtr("/new-host")}},
		{"volume", "volume", `"volumeName":"vol"`, `"volumeName":"new-vol"`, MountArgs{Type: "volume", MountPath: "/data", VolumeName: stringPtr("new-vol")}},
		{"file", "file", `"filePath":"/etc/config","content":"old"`, `"filePath":"/etc/new-config","content":"new"`, MountArgs{Type: "file", MountPath: "/data", FilePath: stringPtr("/etc/new-config"), Content: stringPtr("new")}},
	}
	for _, target := range targets {
		for _, mount := range mounts {
			t.Run(target.name+"/"+mount.name, func(t *testing.T) {
				args := mount.args
				setMountTarget(&args, target.idField, target.id)
				mountRead := fmt.Sprintf(`{"mountId":"m1","mountPath":"/data","type":"%s","serviceType":"%s","%s":"%s",%s}`, mount.typ, target.serviceType, target.idField, target.id, mount.mountFields)
				updateBody := fmt.Sprintf(`{"applicationId":null,"composeId":null,"content":null,"filePath":null,"hostPath":null,"mariadbId":null,"mountId":"m1","mountPath":"/data","mysqlId":null,"postgresId":null,"redisId":null,"serviceType":"%s","type":"%s","volumeName":null}`, target.serviceType, mount.typ)
				updateBody = replaceMountUpdateField(updateBody, target.idField, target.id)
				updateBody = replaceMountUpdateField(updateBody, mount.updateFields, "")
				s := newScriptedServer(t,
					expectPOST("/api/mounts.update", updateBody, `{}`),
					expectGET("/api/mounts.one", map[string][]string{"mountId": {"m1"}}, http.StatusOK, mountRead),
					expectGET(target.statusPath, map[string][]string{target.queryKey: {target.id}}, http.StatusOK, target.statusJSON),
					expectPOST(target.deployPath, target.deployBody, `{}`),
					expectGET(target.statusPath, map[string][]string{target.queryKey: {target.id}}, http.StatusOK, target.statusJSON),
				)
				got, err := (Mount{client: fixedClient(s.API())}).Update(t.Context(), infer.UpdateRequest[MountArgs, MountState]{ID: "m1", Inputs: args})
				require.NoError(t, err)
				require.Equal(t, "m1", got.Output.MountID)
				require.Equal(t, mount.typ, got.Output.Type)
			})
		}
	}
}

func TestMountDeleteBodyAndRedeployMatrix(t *testing.T) {
	targets := mountLifecycleTargetCases()
	mounts := []struct {
		name, typ string
		args      MountArgs
	}{
		{"bind", "bind", MountArgs{Type: "bind", MountPath: "/data", HostPath: stringPtr("/host")}},
		{"volume", "volume", MountArgs{Type: "volume", MountPath: "/data", VolumeName: stringPtr("vol")}},
		{"file", "file", MountArgs{Type: "file", MountPath: "/data", FilePath: stringPtr("/etc/config"), Content: stringPtr("secret")}},
	}
	for _, target := range targets {
		for _, mount := range mounts {
			t.Run(target.name+"/"+mount.name, func(t *testing.T) {
				args := mount.args
				setMountTarget(&args, target.idField, target.id)
				s := newScriptedServer(t,
					expectGET("/api/mounts.one", map[string][]string{"mountId": {"m1"}}, http.StatusOK, `{"mountId":"m1"}`),
					expectPOST("/api/mounts.remove", `{"mountId":"m1"}`, `{}`),
					expectGET(target.statusPath, map[string][]string{target.queryKey: {target.id}}, http.StatusOK, target.statusJSON),
					expectPOST(target.deployPath, target.deployBody, `{}`),
					expectGET(target.statusPath, map[string][]string{target.queryKey: {target.id}}, http.StatusOK, target.statusJSON),
				)
				_, err := (Mount{client: fixedClient(s.API())}).Delete(t.Context(), infer.DeleteRequest[MountState]{ID: "m1", State: MountState{MountArgs: args}})
				require.NoError(t, err)
			})
		}
	}
}

type mountLifecycleTargetCase struct {
	name, serviceType, idField, id, statusPath, queryKey, statusJSON, deployPath, deployBody string
}

func mountLifecycleTargetCases() []mountLifecycleTargetCase {
	return []mountLifecycleTargetCase{
		{"application", "application", "applicationId", "a1", "/api/application.one", "applicationId", `{"applicationId":"a1","applicationStatus":"done"}`, "/api/application.redeploy", `{"applicationId":"a1"}`},
		{"compose", "compose", "composeId", "c1", "/api/compose.one", "composeId", `{"composeId":"c1","composeStatus":"done"}`, "/api/compose.redeploy", `{"composeId":"c1"}`},
		{"postgres", "postgres", "postgresId", "p1", "/api/postgres.one", "postgresId", `{"postgresId":"p1","applicationStatus":"done"}`, "/api/postgres.deploy", `{"postgresId":"p1"}`},
		{"mysql", "mysql", "mysqlId", "my1", "/api/mysql.one", "mysqlId", `{"mysqlId":"my1","applicationStatus":"done"}`, "/api/mysql.deploy", `{"mysqlId":"my1"}`},
		{"mariadb", "mariadb", "mariadbId", "ma1", "/api/mariadb.one", "mariadbId", `{"mariadbId":"ma1","applicationStatus":"done"}`, "/api/mariadb.deploy", `{"mariadbId":"ma1"}`},
		{"redis", "redis", "redisId", "r1", "/api/redis.one", "redisId", `{"redisId":"r1","applicationStatus":"done"}`, "/api/redis.deploy", `{"redisId":"r1"}`},
	}
}

func setMountTarget(args *MountArgs, field, id string) {
	switch field {
	case "applicationId":
		args.ApplicationID = stringPtr(id)
	case "composeId":
		args.ComposeID = stringPtr(id)
	case "postgresId":
		args.PostgresID = stringPtr(id)
	case "mysqlId":
		args.MySQLID = stringPtr(id)
	case "mariadbId":
		args.MariaDBID = stringPtr(id)
	case "redisId":
		args.RedisID = stringPtr(id)
	}
}

func replaceMountUpdateField(body, field, value string) string {
	if value != "" {
		return strings.Replace(body, `"`+field+`":null`, fmt.Sprintf(`"%s":"%s"`, field, value), 1)
	}
	separator := strings.Index(field, `":`)
	if separator < 1 {
		return body
	}
	name := field[1:separator]
	return strings.Replace(body, `"`+name+`":null`, field, 1)
}

func TestMountDeleteRetriesAbsentMountByRedeployingTarget(t *testing.T) {
	s := newScriptedServer(t,
		expectGET("/api/mounts.one", map[string][]string{"mountId": {"m1"}}, http.StatusNotFound, `{}`),
		expectGET("/api/application.one", map[string][]string{"applicationId": {"a1"}}, http.StatusOK, `{"applicationId":"a1","applicationStatus":"done"}`),
		expectPOST("/api/application.redeploy", `{"applicationId":"a1"}`, `{}`),
		expectGET("/api/application.one", map[string][]string{"applicationId": {"a1"}}, http.StatusOK, `{"applicationId":"a1","applicationStatus":"done"}`),
	)
	_, err := (Mount{client: fixedClient(s.API())}).Delete(t.Context(), infer.DeleteRequest[MountState]{ID: "m1", State: MountState{MountArgs: MountArgs{ApplicationID: stringPtr("a1")}}})
	require.NoError(t, err)
}

func TestMountDeleteSkipsRedeployForMissingTarget(t *testing.T) {
	s := newScriptedServer(t,
		expectGET("/api/mounts.one", map[string][]string{"mountId": {"m1"}}, http.StatusNotFound, `{}`),
		expectGET("/api/application.one", map[string][]string{"applicationId": {"a1"}}, http.StatusNotFound, `{}`),
	)
	_, err := (Mount{client: fixedClient(s.API())}).Delete(t.Context(), infer.DeleteRequest[MountState]{ID: "m1", State: MountState{MountArgs: MountArgs{ApplicationID: stringPtr("a1")}}})
	require.NoError(t, err)
}

func TestMountRead404RefreshesAsAbsent(t *testing.T) {
	s := newScriptedServer(t, expectGET("/api/mounts.one", map[string][]string{"mountId": {"m1"}}, http.StatusNotFound, `{}`))
	got, err := (Mount{client: fixedClient(s.API())}).Read(t.Context(), infer.ReadRequest[MountArgs, MountState]{ID: "m1"})
	require.NoError(t, err)
	require.Empty(t, got.ID)
}

func TestMountImportReconstructsEachTypedTarget(t *testing.T) {
	cases := []struct {
		name, id, targetJSON string
		check                func(*testing.T, MountArgs)
	}{
		{"application", "a1", `"applicationId":"a1"`, func(t *testing.T, a MountArgs) { require.Equal(t, "a1", *a.ApplicationID) }},
		{"compose", "c1", `"composeId":"c1"`, func(t *testing.T, a MountArgs) { require.Equal(t, "c1", *a.ComposeID) }},
		{"postgres", "p1", `"postgresId":"p1"`, func(t *testing.T, a MountArgs) { require.Equal(t, "p1", *a.PostgresID) }},
		{"mysql", "my1", `"mysqlId":"my1"`, func(t *testing.T, a MountArgs) { require.Equal(t, "my1", *a.MySQLID) }},
		{"mariadb", "ma1", `"mariadbId":"ma1"`, func(t *testing.T, a MountArgs) { require.Equal(t, "ma1", *a.MariaDBID) }},
		{"redis", "r1", `"redisId":"r1"`, func(t *testing.T, a MountArgs) { require.Equal(t, "r1", *a.RedisID) }},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			s := newScriptedServer(t, expectGET("/api/mounts.one", map[string][]string{"mountId": {"m1"}}, http.StatusOK, `{"mountId":"m1","mountPath":"/data","type":"bind","serviceType":"`+tc.name+`",`+tc.targetJSON+`}`))
			got, err := (Mount{client: fixedClient(s.API())}).Read(t.Context(), infer.ReadRequest[MountArgs, MountState]{ID: "m1"})
			require.NoError(t, err)
			tc.check(t, got.Inputs)
		})
	}
}

func rawJSON(value string) json.RawMessage { return json.RawMessage(value) }
