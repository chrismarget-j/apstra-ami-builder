#!/bin/bash -eu
set -o pipefail

REPOROOT=$(cd $(dirname $0); pwd)
TFROOT=${REPOROOT}/terraform

DOWNLOAD_PAGE="https://support.juniper.net/support/downloads/?p=afc"

MOD="module.upload"
F_LABEL="${MOD}.aws_lambda_function.ours"

echo -n "checking for valid awd credentials...  "
aws sts get-caller-identity > /dev/null
echo "done."

echo -n "fetching and parsing terraform state...  "
STATE=$(terraform -chdir=${TFROOT} show -json)
FUNCTION_NAME=$(jq -r ".values.root_module.child_modules[] | select(.address == \"$MOD\").resources[] | select(.address == \"$F_LABEL\").values.function_name" <<< $STATE)
echo "done."

echo ""
echo "Please visit https://support.juniper.net/support/downloads/?p=afc and click"
echo "the link for \"Apstra VM Image for VMware ESXi\" (an \"ova\" file). Then copy"
echo "tokenized download link and paste it here."
echo ""
echo -n "link: "
read URI

URI_REGEX='^(([^:/?#]+):)?(//((([^:/?#]+)@)?([^:/?#]+)(:([0-9]+))?))?(/([^?#]*))(\?([^#]*))?(#(.*))?'

[[ "$URI" =~ $URI_REGEX ]]
OVA_PATH="${BASH_REMATCH[10]}"
OVA=$(basename $OVA_PATH)

OVA_REGEX='^(aos_server_[0-9.]+-[0-9]+).ova$'
[[ "$OVA" =~ $OVA_REGEX ]]
VMDK="${BASH_REMATCH[1]}-disk1.vmdk"
VMDK_KEY="${BASH_REMATCH[1]}.vmdk"

FILEMAP="{}"
FILEMAP=$(jq -c ".|.[\"$VMDK\"]=\"$VMDK_KEY\"" <<< $FILEMAP)

PAYLOAD="{}"
PAYLOAD=$(jq -c ".|.[\"url\"]=\"$URI\"" <<< $PAYLOAD)
PAYLOAD=$(jq -c ".|.[\"file_map\"]=$FILEMAP" <<< $PAYLOAD)

echo ""
echo -n "Initiating AMI deployment... This usually takes 1-2 minutes.  "
RESULT=$(aws lambda invoke --function-name $FUNCTION_NAME --payload file://<(echo $PAYLOAD) --cli-binary-format raw-in-base64-out --cli-read-timeout 180 /dev/stdout)
echo "done."

echo ""
jq <<< $RESULT
