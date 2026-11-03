/**
 * Report Server parameter query values.
 *
 * Text/string controls submit a present empty string when blank (Desktop
 * binds that as Java ""). Numeric, date, boolean, select, and other typed
 * controls stay omitted when blank so existing null/default semantics hold.
 */
(function (root, factory) {
  var api = factory();
  if (typeof module === 'object' && module.exports) {
    module.exports = api;
  }
  root.JrsReportParams = api;
})(typeof globalThis !== 'undefined' ? globalThis : this, function () {
  'use strict';

  function isTextParameter(type) {
    var t = String(type == null || type === '' ? 'string' : type).toLowerCase();
    return t === 'string' || t === 'text' || t === 'java.lang.string';
  }

  /**
   * Value to put on the report request for one scalar parameter.
   * null means omit the parameter. "" means present and empty.
   *
   * options.preserveBlank — false for select/lookup controls (default true).
   * options.explicitEmpty — the user cleared the control; do not refill a default.
   * options.fallbackDefault — configured default used only when blank and not cleared.
   * options.hidden / options.autoSourced — a blank stays omitted.
   */
  function scalarQueryValue(param, rawValue, options) {
    var opts = options || {};
    var value = rawValue == null ? '' : String(rawValue);
    if (value !== '') {
      return value;
    }
    if (opts.hidden || opts.autoSourced) {
      return null;
    }
    var fallback = opts.fallbackDefault;
    if (!opts.explicitEmpty && fallback != null && String(fallback) !== '') {
      return String(fallback);
    }
    var preserve = opts.preserveBlank !== false && isTextParameter(param && param.type);
    if (preserve) {
      return '';
    }
    return null;
  }

  /** Append name=value, including a present empty string. Null is omitted. */
  function appendScalarToUrl(url, name, value) {
    if (value == null) {
      return url;
    }
    return url + '&' + encodeURIComponent(name) + '=' + encodeURIComponent(String(value));
  }

  return {
    isTextParameter: isTextParameter,
    scalarQueryValue: scalarQueryValue,
    appendScalarToUrl: appendScalarToUrl
  };
});
