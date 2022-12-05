#!/bin/bash -eu
set -o pipefail

REPOROOT=$(cd "$(dirname $0)"; pwd)
TFROOT=${REPOROOT}/terraform

DOWNLOAD_PAGE="https://support.juniper.net/support/downloads/?p=afc"
FUNCTION_NAME_TFSTATE_PATH=".values.outputs.upload_function_name.value"

die() {
  echo "$1"
  exit $2
}

echo -n "Checking for valid aws credentials..."
aws sts get-caller-identity > /dev/null
echo "  Done."

echo -n "Fetching terraform state..."
if ! STATE=$(terraform -chdir=${TFROOT} show -json); then
  die "  Error fetching terraform state - is the project deployed?" $?
fi
echo "  Done."

echo -n "Parsing terraform state..."
FUNCTION_NAME=$(jq -r "$FUNCTION_NAME_TFSTATE_PATH" <<< $STATE)
if [ "$FUNCTION_NAME" == "null" ]; then
  die "  Upload lambda function name not found in terrraform state - is the project deployed?" $?
fi
echo "  Done."

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

read -r URI
if [ "$URI" == "" ]; then
  URI=$DEFAULT_LINK_PROMPT
fi

URI_REGEX='^(([^:/?#]+):)?(//((([^:/?#]+)@)?([^:/?#]+)(:([0-9]+))?))?(/([^?#]*))(\?([^#]*))?(#(.*))?'

[[ "$URI" =~ $URI_REGEX ]] || die "error parsing link" $?
OVA_PATH="${BASH_REMATCH[10]}"
OVA=$(basename $OVA_PATH)
echo $OVA

OVA_REGEX='^(aos_server_[0-9.]+-[0-9]+).ova$'
[[ "$OVA" =~ $OVA_REGEX ]] || die "error parsing ova filename within link" $?
VMDK="${BASH_REMATCH[1]}-disk1.vmdk"
VMDK_KEY="${BASH_REMATCH[1]}.vmdk"

FILEMAP="{}"
FILEMAP=$(jq -c ".|.[\"$VMDK\"]=\"$VMDK_KEY\"" <<< $FILEMAP)

PAYLOAD="{}"
PAYLOAD=$(jq -c ".|.[\"url\"]=\"$URI\"" <<< $PAYLOAD)
PAYLOAD=$(jq -c ".|.[\"file_map\"]=$FILEMAP" <<< $PAYLOAD)

echo ""
echo "Initiating AMI deployment."
echo "File '$VMDK' from the ova will be extracted as '$VMDK_KEY'."
echo -n "This usually takes 1-2 minutes..."
RESULT=$(aws lambda invoke --function-name $FUNCTION_NAME --payload file://<(echo $PAYLOAD) --cli-binary-format raw-in-base64-out --cli-read-timeout 180 /dev/stdout)
echo "  Done."

echo ""
jq <<< $RESULT
