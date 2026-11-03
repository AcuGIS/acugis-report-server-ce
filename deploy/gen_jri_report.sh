#!/bin/bash -e

source /etc/environment

JRI_HOME="${CATALINA_HOME}/jasper_reports/"

# shellcheck source=/dev/null
source "${JRI_HOME}/schedules/${1}_env.sh"

DONT_MAIL="${2}"
export EMAIL='root@localhost'

# ----- Report Builder (Chromium PDF) -----
# ----- Report Builder (Chromium PDF) -----
if [ "${REPORT_SOURCE}" = "builder" ]; then
  if [ "${EMAIL_TEMPLATE}" ]; then
    EMAIL_BODY=$(cat "${JRI_HOME}/email_tmpl/${EMAIL_TEMPLATE}")
  fi

  CHROME_BIN="${REPORT_BUILDER_CHROMIUM_BIN:-/usr/bin/google-chrome}"


  if [ -z "${REPORT_BUILDER_PDF_BASE_URL}" ] || [ -z "${BUILDER_REPORT_ID}" ]; then
    echo "Missing REPORT_BUILDER_PDF_BASE_URL or BUILDER_REPORT_ID in schedule env"
    exit 1
  fi

  if [ -z "${REPORT_PDF_TOKEN}" ]; then
    echo "Missing REPORT_PDF_TOKEN (e.g. in /etc/environment for the service user)"
    exit 1
  fi

  OPT_QUERY=""
  if [ -n "${OPT_PARAMS}" ]; then
    OPT_QUERY="&${OPT_PARAMS}"
  fi

  PDF_URL="${REPORT_BUILDER_PDF_BASE_URL}/report_pdf.php?id=${BUILDER_REPORT_ID}&token=${REPORT_PDF_TOKEN}${OPT_QUERY}"

  TSTAMP=$(date '+%Y%m%d_%H%M%S')
  REPORT_FOLDER="report_builder"
  mkdir -p "${JRI_HOME}/reports/${REPORT_FOLDER}"
  REP_FILEPATH="${JRI_HOME}/reports/${REPORT_FOLDER}/${TSTAMP}_${REP_FILE}"

export HOME=/var/www
export XDG_CONFIG_HOME=/var/www/.config
export XDG_CACHE_HOME=/var/www/.cache

set +e
"${CHROME_BIN}" --headless --no-sandbox --disable-gpu --disable-dev-shm-usage \
  --virtual-time-budget=10000 \
  --print-to-pdf="${REP_FILEPATH}" \
  "${PDF_URL}" 2>&1
_chrome_rc=$?
set -e

  if [ "${_chrome_rc}" -ne 0 ]; then
    echo "Chromium exited with code ${_chrome_rc}"
  fi

  if [ ! -s "${REP_FILEPATH}" ]; then
    echo "Chromium did not produce a PDF at ${REP_FILEPATH}"
    rm -f "${REP_FILEPATH}"
    exit 1
  fi

  if [ -z "${DONT_MAIL}" ]; then
    echo "${EMAIL_BODY}" | mutt -F /var/www/.muttrc -e "set content_type=text/html" -s "${EMAIL_SUBJ}" -a "${REP_FILEPATH}" -- ${RECP_EMAIL}
  fi

  exit 0
fi
# ----- Jasper (wget) -----
if [ "${EMAIL_TEMPLATE}" ]; then
  EMAIL_BODY=$(cat "${JRI_HOME}/email_tmpl/${EMAIL_TEMPLATE}")
fi
REPORT_FOLDER=$(dirname "${REP_ID}")

#encode the / in report id
REP_ID=$(echo "${REP_ID}" | sed 's/\//%2F/g')

if [ "${OPT_PARAMS}" ]; then
  OPT_PARAMS="&${OPT_PARAMS}"
fi

URL="http://localhost:8080/acugis-report-server/report?_repName=${REP_ID}&_repFormat=${REP_FORMAT}&_dataSource=${REP_DATASOURCE}&_outFilename=${REP_FILE}${OPT_PARAMS}"

TSTAMP=$(date '+%Y%m%d_%H%M%S')
REP_FILEPATH="${JRI_HOME}/reports/${REPORT_FOLDER}/${TSTAMP}_${REP_FILE}"

wget -O"${REP_FILEPATH}" "${URL}"
if [ $? -ne 0 ]; then
  rm -f "${REP_FILEPATH}"
fi

if [ -z "${DONT_MAIL}" ]; then
  echo "${EMAIL_BODY}" | mutt -F /var/www/.muttrc -e "set content_type=text/html" -s "${EMAIL_SUBJ}" -a "${REP_FILEPATH}" -- ${RECP_EMAIL}
fi

exit 0
