#!/bin/bash -eu
set -o pipefail

REPOROOT=$(cd "$(dirname $0)"; pwd)
TFROOT=${REPOROOT}/terraform

DOWNLOAD_PAGE="https://support.juniper.net/support/downloads/?p=afc"
FUNCTION_NAME_TFSTATE_PATH=".values.outputs.upload_function_name.value"

URI_REGEX='^(([^:/?#]+):)?(//((([^:/?#]+)@)?([^:/?#]+)(:([0-9]+))?))?(/([^?#]*))(\?([^#]*))?(#(.*))?'

die() {
  echo "$1"
  exit $2
}

remove_query_string() {
  [[ "$1" =~ $URI_REGEX ]] || die "error parsing link" $?

  # extract the query string length so we can trim it off
  if [ ${#BASH_REMATCH[@]} -ge 13 ]; then
    TRIM="${#BASH_REMATCH[12]}"
  else
    TRIM=0
  fi

  SHORT_URI="${URI:0:((${#URI}-$TRIM))}"
}

check_aws_creds() {
  echo -n "Checking for valid aws credentials..."
  aws sts get-caller-identity > /dev/null
  echo "  Done."
}

fetch_tf_state() {
  echo -n "Fetching terraform state..."
  if ! STATE=$(terraform -chdir=${TFROOT} show -json); then
    die "  Error fetching terraform state - is the project deployed?" $?
  fi
  echo "  Done."
}

parse_tf_state() {
  echo -n "Parsing terraform state..."
  FUNCTION_NAME=$(jq -r "$FUNCTION_NAME_TFSTATE_PATH" <<< $STATE)
  if [ "$FUNCTION_NAME" == "null" ]; then
    die "  Upload lambda function name not found in terrraform state - is the project deployed?" $?
  fi
  echo "  Done."
}

prompt_for_link() {
  echo ""
  echo "Please visit $DOWNLOAD_PAGE and click"
  echo "the link for \"Apstra VM Image for VMware ESXi\" (an \"ova\" file). Then copy"
  echo "tokenized download link and paste it here."
  echo ""
  if DEFAULT_LINK_PROMPT=$(printenv APSTRA_DOWNLOAD_LINK); then
    echo -n "Download link [$DEFAULT_LINK_PROMPT]: "
  else
    echo -n "Download link: "
  fi
}

read_uri() {
  read -r URI
  if [ "$URI" == "" ]; then
    URI=$DEFAULT_LINK_PROMPT
  fi

  [[ "$URI" =~ $URI_REGEX ]] || die "error parsing link" $?
  OVA_PATH="${BASH_REMATCH[10]}"
  OVA=$(basename $OVA_PATH)

  remove_query_string "$URI"
}

read_ova_filename() {
  OVA_REGEX='^(aos_server_[0-9.]+-[0-9]+).ova$'
  [[ "$OVA" =~ $OVA_REGEX ]] || die "error parsing ova filename within link" $?
  VMDK="${BASH_REMATCH[1]}-disk1.vmdk"

  APSTRA_VERSION_REGEX='^aos_server_([0-9.]+)-([0-9]+).ova$'
  [[ "$OVA" =~ $APSTRA_VERSION_REGEX ]] || die "error parsing ova version within link" $?
  VERSION=${BASH_REMATCH[1]}
  BUILD=${BASH_REMATCH[2]}
}

make_tag() {
  TAG="{}"
  TAG=$(jq -c ".|.[\"key\"]=\"$1\"" <<< $TAG)
  TAG=$(jq -c ".|.[\"value\"]=\"$2\"" <<< $TAG)
  echo $TAG
}

check_aws_creds
fetch_tf_state
parse_tf_state
prompt_for_link
read_uri
read_ova_filename

URITAG=$(make_tag "url" "$SHORT_URI")
VERTAG=$(make_tag "version" "$VERSION")
BUILDTAG=$(make_tag "build" "$BUILD")
NAMETAG=$(make_tag "Name" "apstra $VERSION")
CITAG=$(make_tag "cloud-init" "false")

TAGS=$(jq -s . <<< "$URITAG $VERTAG $BUILDTAG $NAMETAG $CITAG")

S3OBJINFO="{}"
S3OBJINFO=$(jq -c ".|.[\"src\"]=\"$VMDK\"" <<< $S3OBJINFO)
S3OBJINFO=$(jq -c ".|.[\"dst\"]=\"$VMDK\"" <<< $S3OBJINFO)
S3OBJINFO=$(jq -c ".|.[\"tags\"]=$TAGS" <<< $S3OBJINFO)

FILES=$(jq -s . <<< $S3OBJINFO)

REQUEST="{}"
REQUEST=$(jq -c ".|.[\"url\"]=\"$URI\"" <<< $REQUEST)
REQUEST=$(jq -c ".|.[\"files\"]=$FILES" <<< $REQUEST)

echo ""
echo "Initiating AMI deployment."
echo "File '$VMDK' from the ova will be extracted as '$VMDK'."
echo -n "This usually takes 1-2 minutes..."
RESULT=$(aws lambda invoke --function-name $FUNCTION_NAME --payload file://<(echo $REQUEST) --cli-binary-format raw-in-base64-out --cli-read-timeout 180 /dev/stdout)
echo "  Done."

echo ""
jq <<< $RESULT

echo "check status with:"

for task_id in $(jq -r '.task_ids | select (. != null) | .[]' <<< $RESULT)
do
  echo "  aws ec2 describe-import-snapshot-tasks --import-task-ids $task_id"
done
