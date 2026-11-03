using AcuGIS.Report.Errors;

namespace AcuGIS.Report.Services;

/// <summary>Reserved surface for future Users REST API.</summary>
public sealed class UsersService
{
    internal UsersService()
    {
    }

    public Task<object> ListAsync(CancellationToken cancellationToken = default) =>
        Task.FromException<object>(new NotImplementedByServerException("Users"));
}

/// <summary>Reserved surface for future Groups REST API.</summary>
public sealed class GroupsService
{
    internal GroupsService()
    {
    }

    public Task<object> ListAsync(CancellationToken cancellationToken = default) =>
        Task.FromException<object>(new NotImplementedByServerException("Groups"));
}

/// <summary>Reserved surface for future Schedules REST API.</summary>
public sealed class SchedulesService
{
    internal SchedulesService()
    {
    }

    public Task<object> ListAsync(CancellationToken cancellationToken = default) =>
        Task.FromException<object>(new NotImplementedByServerException("Schedules"));
}

/// <summary>Reserved surface for future Jobs REST API.</summary>
public sealed class JobsService
{
    internal JobsService()
    {
    }

    public Task<object> ListAsync(CancellationToken cancellationToken = default) =>
        Task.FromException<object>(new NotImplementedByServerException("Jobs"));
}

/// <summary>Reserved surface for future Recipients REST API.</summary>
public sealed class RecipientsService
{
    internal RecipientsService()
    {
    }

    public Task<object> ListAsync(CancellationToken cancellationToken = default) =>
        Task.FromException<object>(new NotImplementedByServerException("Recipients"));
}
