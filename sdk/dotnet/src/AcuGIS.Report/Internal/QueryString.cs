using System.Globalization;
using System.Text;

namespace AcuGIS.Report.Internal;

internal static class QueryString
{
    public static string Build(string path, IEnumerable<(string Key, string? Value)>? parameters = null)
    {
        if (parameters is null)
        {
            return path;
        }

        var sb = new StringBuilder(path);
        var first = !path.Contains('?', StringComparison.Ordinal);
        foreach (var (key, value) in parameters)
        {
            if (value is null)
            {
                continue;
            }

            sb.Append(first ? '?' : '&');
            first = false;
            sb.Append(Uri.EscapeDataString(key));
            sb.Append('=');
            sb.Append(Uri.EscapeDataString(value));
        }

        return sb.ToString();
    }

    public static IEnumerable<(string Key, string? Value)> FromReportParameters(
        IEnumerable<KeyValuePair<string, object?>>? parameters)
    {
        if (parameters is null)
        {
            yield break;
        }

        foreach (var kv in parameters)
        {
            if (kv.Value is null)
            {
                continue;
            }

            if (kv.Value is string s)
            {
                yield return (kv.Key, s);
                continue;
            }

            if (kv.Value is System.Collections.IEnumerable enumerable and not string)
            {
                foreach (var item in enumerable)
                {
                    if (item is null)
                    {
                        continue;
                    }

                    yield return (kv.Key, Convert.ToString(item, CultureInfo.InvariantCulture));
                }

                continue;
            }

            yield return (kv.Key, Convert.ToString(kv.Value, CultureInfo.InvariantCulture));
        }
    }

    public static string Format(bool? value) =>
        value switch
        {
            true => "true",
            false => "false",
            null => "",
        };

    public static string Format(long? value) =>
        value?.ToString(CultureInfo.InvariantCulture) ?? "";

    public static string Format(int? value) =>
        value?.ToString(CultureInfo.InvariantCulture) ?? "";

    public static string Format(double? value) =>
        value?.ToString(CultureInfo.InvariantCulture) ?? "";
}
