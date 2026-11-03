using System.Net;

namespace AcuGIS.Report.Errors;

/// <summary>Base exception for AcuGIS Report Server client errors.</summary>
public class ReportServerException : Exception
{
    public ReportServerException(string message, Exception? inner = null)
        : base(message, inner)
    {
    }

    /// <summary>HTTP status code when available.</summary>
    public HttpStatusCode? StatusCode { get; init; }

    /// <summary>Raw response body from the server (JSON or plain text).</summary>
    public string? ResponseBody { get; init; }

    /// <summary>Optional machine-readable error code from the server.</summary>
    public string? ErrorCode { get; init; }

    /// <summary>Optional hint from the server.</summary>
    public string? Hint { get; init; }
}

/// <summary>401 — authentication required or invalid credentials.</summary>
public sealed class AuthenticationException : ReportServerException
{
    public AuthenticationException(string message, Exception? inner = null)
        : base(message, inner)
    {
    }
}

/// <summary>403 — permission denied.</summary>
public sealed class PermissionDeniedException : ReportServerException
{
    public PermissionDeniedException(string message, Exception? inner = null)
        : base(message, inner)
    {
    }
}

/// <summary>400 / 422 — validation or bad request.</summary>
public sealed class ValidationException : ReportServerException
{
    public ValidationException(string message, Exception? inner = null)
        : base(message, inner)
    {
    }

    /// <summary>Field-level validation messages when present.</summary>
    public IReadOnlyList<string> Errors { get; init; } = Array.Empty<string>();
}

/// <summary>404 — resource not found.</summary>
public sealed class NotFoundException : ReportServerException
{
    public NotFoundException(string message, Exception? inner = null)
        : base(message, inner)
    {
    }
}

/// <summary>409 — conflict (e.g. folder not empty, name clash).</summary>
public sealed class ConflictException : ReportServerException
{
    public ConflictException(string message, Exception? inner = null)
        : base(message, inner)
    {
    }
}

/// <summary>5xx — server or dependency failure.</summary>
public sealed class ServerException : ReportServerException
{
    public ServerException(string message, Exception? inner = null)
        : base(message, inner)
    {
    }
}

/// <summary>Transport-level failure (DNS, connection reset, TLS, etc.).</summary>
public sealed class NetworkException : ReportServerException
{
    public NetworkException(string message, Exception? inner = null)
        : base(message, inner)
    {
    }
}

/// <summary>Request exceeded the configured timeout.</summary>
public sealed class TimeoutException : ReportServerException
{
    public TimeoutException(string message, Exception? inner = null)
        : base(message, inner)
    {
    }
}

/// <summary>Response could not be parsed as expected.</summary>
public sealed class InvalidResponseException : ReportServerException
{
    public InvalidResponseException(string message, Exception? inner = null)
        : base(message, inner)
    {
    }
}

/// <summary>
/// Thrown for reserved service surfaces (Users, Groups, Schedules, Jobs, Recipients)
/// that are not yet implemented by the server REST API.
/// </summary>
public sealed class NotImplementedByServerException : ReportServerException
{
    public NotImplementedByServerException(string feature)
        : base($"{feature} is not implemented by the AcuGIS Report Server REST API yet.")
    {
        Feature = feature;
    }

    public string Feature { get; }
}
