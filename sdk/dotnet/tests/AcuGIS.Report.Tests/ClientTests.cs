using System.Net;
using System.Text;
using AcuGIS.Report.Errors;
using AcuGIS.Report.Models;
using Xunit;

namespace AcuGIS.Report.Tests;

public class ClientTests
{
    private static ReportServerClient CreateClient(MockHttpHandler handler, ClientOptions? options = null)
    {
        options ??= new ClientOptions
        {
            BaseUrl = "https://reports.example.com",
            Token = "jrs_testtoken",
        };
        var http = new HttpClient(handler) { BaseAddress = new Uri("https://reports.example.com/") };
        return new ReportServerClient(http, options);
    }

    [Fact]
    public async Task Auth_Me_SendsBearerAndUserAgent()
    {
        var handler = new MockHttpHandler(req =>
        {
            Assert.Equal(HttpMethod.Get, req.Method);
            Assert.Equal("/api/v1/auth/me", req.RequestUri!.AbsolutePath);
            Assert.Equal("Bearer", req.Headers.Authorization?.Scheme);
            Assert.Equal("jrs_testtoken", req.Headers.Authorization?.Parameter);
            Assert.True(req.Headers.TryGetValues("User-Agent", out var ua));
            Assert.Contains("AcuGIS.Report/0.1.0", string.Join(' ', ua));
            return MockHttpHandler.Json("""{"id":1,"username":"admin","is_admin":true,"groups":["Administrators"]}""");
        });

        using var client = CreateClient(handler);
        var me = await client.Auth.MeAsync();
        Assert.Equal("admin", me.Username);
        Assert.True(me.IsAdmin);
        Assert.Contains("Administrators", me.Groups!);
    }

    [Fact]
    public async Task Reports_ListAsync()
    {
        var handler = new MockHttpHandler(_ => MockHttpHandler.Json(
            """{"count":1,"items":[{"key":"invoice","name":"Invoice","type":"jasper","has_thumbnail":true}]}"""));

        using var client = CreateClient(handler);
        var list = await client.Reports.ListAsync();
        Assert.Equal(1, list.Count);
        Assert.Equal("invoice", list.Items![0].Key);
    }

    [Fact]
    public async Task Reports_Run_BinaryStream()
    {
        var pdf = Encoding.UTF8.GetBytes("%PDF-1.4 fake");
        var handler = new MockHttpHandler(req =>
        {
            Assert.Equal("/reports/execute", req.RequestUri!.AbsolutePath);
            var query = req.RequestUri.Query;
            Assert.Contains("_repName=Invoice", query);
            Assert.Contains("_repFormat=pdf", query);
            return MockHttpHandler.Bytes(pdf);
        });

        using var client = CreateClient(handler);
        await using var stream = await client.Reports.RunStreamAsync("Invoice", "pdf");
        using var ms = new MemoryStream();
        await stream.CopyToAsync(ms);
        Assert.Equal(pdf, ms.ToArray());
    }

    [Fact]
    public async Task Reports_Run_MultiValueParameters()
    {
        var handler = new MockHttpHandler(req =>
        {
            var query = req.RequestUri!.Query;
            Assert.Contains("city=NYC", query);
            Assert.Contains("city=LA", query);
            Assert.Contains("year=2024", query);
            return MockHttpHandler.Bytes(new byte[] { 1, 2, 3 });
        });

        using var client = CreateClient(handler);
        var parameters = new Dictionary<string, object?>
        {
            ["city"] = new[] { "NYC", "LA" },
            ["year"] = 2024,
        };
        var bytes = await client.Reports.RunAsync("Invoice", "pdf", parameters);
        Assert.Equal(3, bytes.Length);
    }

    [Fact]
    public async Task Reports_RunToFileAsync()
    {
        var path = Path.Combine(Path.GetTempPath(), $"acugis-test-{Guid.NewGuid():N}.pdf");
        try
        {
            var handler = new MockHttpHandler(_ => MockHttpHandler.Bytes(Encoding.UTF8.GetBytes("pdf-bytes")));
            using var client = CreateClient(handler);
            await client.Reports.RunToFileAsync("Invoice", null, path);
            Assert.Equal("pdf-bytes", await File.ReadAllTextAsync(path));
        }
        finally
        {
            if (File.Exists(path))
            {
                File.Delete(path);
            }
        }
    }

    [Fact]
    public async Task Reports_Publish_Multipart()
    {
        var handler = new MockHttpHandler(async (req, ct) =>
        {
            Assert.Equal(HttpMethod.Post, req.Method);
            Assert.Equal("/api/v1/reports", req.RequestUri!.AbsolutePath);
            Assert.StartsWith("multipart/", req.Content!.Headers.ContentType!.MediaType!);
            var body = await req.Content.ReadAsStringAsync(ct);
            Assert.Contains("jrxml_file", body);
            Assert.Contains("data_source", body);
            Assert.Contains("reporting_db", body);
            return MockHttpHandler.Json(
                """{"ok":true,"key":"invoice","message":"Report published"}""",
                HttpStatusCode.Created);
        });

        using var client = CreateClient(handler);
        await using var jrxml = new MemoryStream(Encoding.UTF8.GetBytes("<jasperReport/>"));
        var result = await client.Reports.PublishAsync(new PublishReportRequest
        {
            JrxmlFile = jrxml,
            JrxmlFileName = "invoice.jrxml",
            DataSource = "reporting_db",
            ReportName = "Invoice",
        });
        Assert.Equal("invoice", result.Key);
        Assert.True(result.Ok);
    }

    [Fact]
    public async Task Folders_TreeAndBrowse()
    {
        var handler = new MockHttpHandler(req =>
        {
            if (req.RequestUri!.AbsolutePath.EndsWith("/tree"))
            {
                return MockHttpHandler.Json("""[{"id":1,"name":"Home","children":[]}]""");
            }

            Assert.Contains("folder_id=0", req.RequestUri.Query);
            return MockHttpHandler.Json("""{"folders":[],"items":[],"breadcrumbs":[]}""");
        });

        using var client = CreateClient(handler);
        var tree = await client.Folders.GetTreeAsync();
        Assert.Equal("Home", tree[0].Name);
        var browse = await client.Folders.BrowseAsync(0);
        Assert.NotNull(browse.Items);
    }

    [Fact]
    public async Task Resources_ListAndDownloadToFile()
    {
        var uuid = Guid.Parse("550e8400-e29b-41d4-a716-446655440000");
        var path = Path.Combine(Path.GetTempPath(), $"acugis-res-{Guid.NewGuid():N}.bin");
        try
        {
            var handler = new MockHttpHandler(req =>
            {
                if (req.RequestUri!.AbsolutePath == "/api/v1/resources")
                {
                    return MockHttpHandler.Json(
                        $$"""{"items":[{"uuid":"{{uuid}}","display_name":"logo","resource_type":"image"}]}""");
                }

                Assert.Contains($"/resources/{uuid:D}/download", req.RequestUri.AbsolutePath);
                return MockHttpHandler.Bytes(Encoding.UTF8.GetBytes("image-bytes"), "image/png");
            });

            using var client = CreateClient(handler);
            var list = await client.Resources.ListAsync(resourceType: "image");
            Assert.Equal(uuid, list.Items![0].Uuid);
            await client.Resources.DownloadToFileAsync(uuid, path);
            Assert.Equal("image-bytes", await File.ReadAllTextAsync(path));
        }
        finally
        {
            if (File.Exists(path))
            {
                File.Delete(path);
            }
        }
    }

    [Fact]
    public async Task Errors_MapStatusCodes_AndPreserveBody()
    {
        async Task AssertMaps(HttpStatusCode status, Type exceptionType, string body)
        {
            var handler = new MockHttpHandler(_ => MockHttpHandler.Json(body, status));
            using var client = CreateClient(handler);
            var ex = await Assert.ThrowsAsync(exceptionType, () => client.Auth.MeAsync());
            var typed = Assert.IsAssignableFrom<ReportServerException>(ex);
            Assert.Equal(status, typed.StatusCode);
            Assert.Equal(body, typed.ResponseBody);
        }

        await AssertMaps(HttpStatusCode.Unauthorized, typeof(AuthenticationException),
            """{"error":"authentication required"}""");
        await AssertMaps(HttpStatusCode.Forbidden, typeof(PermissionDeniedException),
            """{"error":"Permission denied"}""");
        await AssertMaps(HttpStatusCode.NotFound, typeof(NotFoundException),
            """{"error":"not found"}""");
        await AssertMaps(HttpStatusCode.Conflict, typeof(ConflictException),
            """{"error":"conflict"}""");
        await AssertMaps(HttpStatusCode.BadRequest, typeof(ValidationException),
            """{"error":"validation failed","errors":["name is required"]}""");
        await AssertMaps(HttpStatusCode.InternalServerError, typeof(ServerException),
            """{"error":"boom"}""");
    }

    [Fact]
    public async Task Timeout_ThrowsTimeoutException()
    {
        var handler = new MockHttpHandler(async (_, ct) =>
        {
            await Task.Delay(TimeSpan.FromSeconds(5), ct);
            return MockHttpHandler.Json("{}");
        });

        var options = new ClientOptions
        {
            BaseUrl = "https://reports.example.com",
            Token = "jrs_x",
            Timeout = TimeSpan.FromMilliseconds(50),
        };
        var http = new HttpClient(handler)
        {
            BaseAddress = new Uri("https://reports.example.com/"),
            Timeout = TimeSpan.FromMilliseconds(50),
        };
        using var client = new ReportServerClient(http, options);
        await Assert.ThrowsAsync<Errors.TimeoutException>(() => client.Health.GetHealthAsync());
    }

    [Fact]
    public async Task Reserved_Users_ThrowsNotImplementedByServer()
    {
        using var client = CreateClient(new MockHttpHandler(_ => MockHttpHandler.Json("{}")));
        var ex = await Assert.ThrowsAsync<NotImplementedByServerException>(() => client.Users.ListAsync());
        Assert.Equal("Users", ex.Feature);
    }

    [Fact]
    public async Task CustomHeaders_AreSent()
    {
        var handler = new MockHttpHandler(req =>
        {
            Assert.True(req.Headers.TryGetValues("X-Request-Id", out var values));
            Assert.Equal("abc-123", Assert.Single(values));
            return MockHttpHandler.Json("""{"status":"ok","service":"acugis-report-server-go"}""");
        });

        var options = new ClientOptions
        {
            BaseUrl = "https://reports.example.com",
            Token = "jrs_x",
            DefaultHeaders = { ["X-Request-Id"] = "abc-123" },
        };
        using var client = CreateClient(handler, options);
        var health = await client.Health.GetHealthAsync();
        Assert.Equal("ok", health.Status);
    }

    [Fact]
    public void FromEnvironment_RequiresUrl()
    {
        var prevUrl = Environment.GetEnvironmentVariable(ClientOptions.UrlEnvironmentVariable);
        var prevToken = Environment.GetEnvironmentVariable(ClientOptions.TokenEnvironmentVariable);
        try
        {
            Environment.SetEnvironmentVariable(ClientOptions.UrlEnvironmentVariable, null);
            Environment.SetEnvironmentVariable(ClientOptions.TokenEnvironmentVariable, null);
            Assert.Throws<InvalidOperationException>(ClientOptions.FromEnvironment);

            Environment.SetEnvironmentVariable(ClientOptions.UrlEnvironmentVariable, "https://reports.example.com");
            Environment.SetEnvironmentVariable(ClientOptions.TokenEnvironmentVariable, "jrs_env");
            var opts = ClientOptions.FromEnvironment();
            Assert.Equal("https://reports.example.com", opts.BaseUrl);
            Assert.Equal("jrs_env", opts.Token);
        }
        finally
        {
            Environment.SetEnvironmentVariable(ClientOptions.UrlEnvironmentVariable, prevUrl);
            Environment.SetEnvironmentVariable(ClientOptions.TokenEnvironmentVariable, prevToken);
        }
    }
}
