#!/bin/bash -eu
set -o pipefail

# breadcrumbs are details left behind by cloud-init
BREADCRUMBS="/var/local/ci-breadcrumbs.json"
if ! [ -r "$BREADCRUMBS" ]
then
  exit
fi

OVA_URI=$(jq -r ".ova_url" < "$BREADCRUMBS")

# attach point for new volume
DEVICE=/dev/sdd

# aos filesystems
AOS_BLK_DEV_PATH="/dev/aos-server-vg"
ROOT_BLK_DEV="$AOS_BLK_DEV_PATH/root"
VAR_BLK_DEV="$AOS_BLK_DEV_PATH/var"
ROOT_MNT_POINT="/mnt"
VAR_MNT_POINT="/mnt/var"

# scratchpad directory
TMP_DIR=$(mktemp -d)

# useful when parsing URIs
URI_REGEX='^(([^:/?#]+):)?(//((([^:/?#]+)@)?([^:/?#]+)(:([0-9]+))?))?(/([^?#]*))(\?([^#]*))?(#(.*))?'

die() {
  echo "die: $i"
  if [ -n "$VOL_INFO" ]
  then
    umount_all
    detach_volume
  fi
  exit 1
}

remove_query_string() {
  URI=$1
  [[ "$URI" =~ $URI_REGEX ]] || die "error parsing link" $?

  # extract the query string length so we can trim it off
  if [ ${#BASH_REMATCH[@]} -ge 13 ]; then
    TRIM="${#BASH_REMATCH[12]}"
  else
    TRIM=0
  fi

  SHORT_URI="${URI:0:((${#URI}-$TRIM))}"
  echo $SHORT_URI
}

setup_aws_stuff() {
  TOKEN=$(curl -sX PUT "http://169.254.169.254/latest/api/token" -H "X-aws-ec2-metadata-token-ttl-seconds: 21600")
  IDENTITY=$(curl -s -H "X-aws-ec2-metadata-token: $TOKEN" "http://169.254.169.254/latest/dynamic/instance-identity/document")
  AZ=$(jq -r '.availabilityZone' <<< "$IDENTITY")
  REGION=$(jq -r '.region' <<< "$IDENTITY")
  INSTANCE_ID=$(jq -r '.instanceId' <<< "$IDENTITY")
}

fetch_image() {
  VMDK_FILE="${TMP_DIR}/$(basename $(remove_query_string $OVA_URI) | sed 's/.ova/-disk1.vmdk/')"
  (cd "$TMP_DIR"; curl -s "${OVA_URI}" | tar xf - "$(basename $VMDK_FILE)")
}

new_volume() {
  SIZEB=$(qemu-img info -f vmdk $VMDK_FILE --output=json | jq -r '."virtual-size"')
  SIZEG=$((($SIZEB/1024/1024/1024)+1))
  VOL_INFO=$(aws ec2 create-volume --region "$REGION" --availability-zone "$AZ" --size "$SIZEG" --volume-type gp3 --iops 3000)
  VOLUME_ID=$(jq -r '.VolumeId' <<< "$VOL_INFO")
  while true
  do
    VOLUME_DESCRIPTION=$(aws ec2 describe-volumes --volume-ids $VOLUME_ID --region $REGION)
    VOLUME_STATE=$(jq -r ".Volumes[0].State" <<< "$VOLUME_DESCRIPTION")
    echo $VOLUME_STATE
    if [ "$VOLUME_STATE" == "available" ]
    then
      break
    fi
  done
}

delete_volume() {
  aws ec2 delete-volume --region "$REGION" --volume-id $VOLUME_ID
}

detach_volume() {
  aws ec2 detach-volume --volume-id "$VOLUME_ID" --region "$REGION"
  wait_attach_status detached
}

wait_attach_status() {
  i=0
  while [ $i -lt 10 ]
  do
    ((i=i+1))
    EBS_INFO=$(aws ec2 describe-instances --instance-id $INSTANCE_ID --region $REGION | jq -r ".Reservations[0].Instances[0].BlockDeviceMappings[]")
    ATTACH_STATUS=$(jq -r "select(.DeviceName==\"$DEVICE\").Ebs.Status" <<< "$EBS_INFO")
    if [ -z "$ATTACH_STATUS" ]
    then
      ATTACH_STATUS="detached"
    fi
    echo status: $ATTACH_STATUS
    if [ "$ATTACH_STATUS" == "$1" ]
    then
      break
    fi
    sleep 1
  done
}

attach_volume() {
  aws ec2 attach-volume --region "$REGION" --instance-id "$INSTANCE_ID" --volume-id "$VOLUME_ID" --device=$DEVICE
  wait_attach_status attached
}

find_new_disk() {
  local i=0
  local max_iterations=10
  local AFTER=$(lsblk | grep disk | awk '{print $1}')
  while [ "$1" == "$AFTER" ] && [ $i -lt $max_iterations ]
  do
    ((i=i+1))
    echo sleeping
    sleep 1
    AFTER=$(lsblk | grep disk | awk '{print $1}')
  done

  for disk in $AFTER
  do
    if ! grep -E "^${disk}$" <<< "$BEFORE" > /dev/null
    then
      local found=$disk
      break
    fi
  done

  if [ -n "$found" ]
  then
    echo "/dev/$found"
  else
    die "new device didn't appear after $i tries"
  fi
}

extract_vmdk() {
  qemu-img convert -p -O raw -f vmdk "$VMDK_FILE" "$1"
}

wait_aos_partition() {
  local max_tries=10
  local i=0
  while [ $i -lt $max_tries ]
  do
    ((i=i+1))
    if [ -b "$1" ]
    then
      break
    fi
    sleep .2
  done

  if [ ! -b "$1" ]
  then
    die "block device $1 does not exist"
  fi
}

wait_snapshot_state() {
  local snapshot=$1
  local state=$2
  i=0
  while [ $i -lt 7200 ]
  do
    ((i=i+1))
    local snapshot_info=$(aws ec2 describe-snapshots --snapshot-ids "$snapshot" --region "$REGION")
    local snapshot_status=$(jq -r ".Snapshots[0].State" <<< "$snapshot_info")
    local snapshot_progress=$(jq -r ".Snapshots[0].Progress" <<< "$snapshot_info")
    if [ -z "$snapshot_status" ]
    then
      snapshot_status="unknown"
    fi
    echo "snapshot status: $snapshot_status $snapshot_progress"
    if [ "$snapshot_status" == "$2" ]
    then
      break
    fi
    sleep 1
  done
}

check_version() {
  local linenum
  case $2 in
    before|between)
      linenum=2;;
    after)
      linenum=1;;
  esac

  if [ "$(printf "%s\n%s\n%s\n" $1 $3 $4 | sort -Vr | sed "${linenum}q;d")" == "$1" ]
  then
    true
  else
    false
  fi
}

umount_all() {
  mount | egrep "${ROOT_MNT_POINT} |${ROOT_MNT_POINT}/" | awk '{print $3}' | sort -r | while read path
  do
    umount $path
  done
}

wait_pvol() {
  local pvolinfo
  i=0
  while [ $i -lt 10 ];do
    ((i=i+1))
    pvolinfo="$(pvs | grep ${DISK}p)"
    if [ -n "$pvolinfo" ]; then
      break
      sleep 1
    fi
  done

  [ -n "$pvolinfo" ] || die "failed to find pvol info for $1"
}

aos_fixup() {
  if [ -z "$1" ]; then
    set 1
  fi

  vgscan --mknodes --notify-dbus
  wait_pvol "$DISK"

  ROOTDEV=$(blkid | grep "${DISK}p" | grep "PARTLABEL=\"apstra_${1}_root\"" | awk '{print $1}' | sed 's/://')
  mount "$ROOTDEV" "$ROOT_MNT_POINT"
  grep '^/dev/disk/by-id/dm-uuid-LVM-' "${ROOT_MNT_POINT}/etc/fstab" | while read -r dev mountpoint fstype opts _ _; do
    mount "$dev" "${ROOT_MNT_POINT}${mountpoint}"
  done

  chage -R "$ROOT_MNT_POINT" -d -1 admin
  sed -i 's/^\(-A INPUT.*-p tcp.*\)/#unsafe-default \1/' ${ROOT_MNT_POINT}/etc/iptables/rules.v4
  sed -i 's/^\(-A INPUT.*-p tcp.*\)/#unsafe-default \1/' ${ROOT_MNT_POINT}/etc/iptables/rules.v6

  umount_all
}

aos_fixup_60x_and_earlier() {
  vgscan --mknodes --notify-dbus

  wait_aos_partition "$ROOT_BLK_DEV"
  mount "$ROOT_BLK_DEV" "$ROOT_MNT_POINT"

  wait_aos_partition "$VAR_BLK_DEV"
  mount "$VAR_BLK_DEV" "$VAR_MNT_POINT"

  chage -R "$ROOT_MNT_POINT" -d -1 admin

  mkdir -p "$ROOT_MNT_POINT/run/systemd/resolve"
  touch "$ROOT_MNT_POINT/run/systemd/resolve/resolv.conf"
  mount -o ro,bind /run/systemd/resolve/resolv.conf "$ROOT_MNT_POINT/run/systemd/resolve/resolv.conf"

  chroot "$ROOT_MNT_POINT" apt-get update -y
  chroot "$ROOT_MNT_POINT" apt-get install -y cloud-init
  rm -f "${ROOT_MNT_POINT}/etc/cloud/cloud-init.disabled"

  umount "$ROOT_MNT_POINT/run/systemd/resolve/resolv.conf"
  rm -rf "$ROOT_MNT_POINT/run/systemd"

  sed -i 's/^\(-A INPUT.*-p tcp.*\)/#unsafe-default \1/' ${ROOT_MNT_POINT}/etc/iptables/rules.v4

  umount_all
}

snapshot() {
  local snap_info

  snap_info=$(aws ec2 create-snapshot --volume-id "$1" --region "$REGION")
  SNAP_ID=$(jq -r ".SnapshotId" <<< "$snap_info")
  wait_snapshot_state "$SNAP_ID" "completed"
  aws ec2 create-tags --region "$REGION" --resources "$SNAP_ID" --tags "Key=Name,Value=$2"
}

register_image() {
  local version
  local build
  local image_info
  local image_id

  version=$(sed 's/-.*$//' <<< "$VERSION")
  build=$(sed 's/^.*-//' <<< "$VERSION")
  image_info=$(aws ec2 register-image \
    --region "$REGION" \
    --description "Apstra with cloud-init" \
    --name "Apstra $VERSION" \
    --ena-support \
    --imds-support "v2.0" \
    --root-device-name "/dev/sda1" \
    --virtualization-type "hvm" \
    --architecture x86_64 \
    --block-device-mappings "DeviceName=/dev/sda1,Ebs={SnapshotId=$SNAP_ID,DeleteOnTermination=true}")

  image_id=$(jq -r '.ImageId' <<< "$image_info")
  aws ec2 create-tags --region "$REGION" --resources "$image_id" --tags "Key=Name,Value=Apstra $VERSION"
  aws ec2 create-tags --region "$REGION" --resources "$image_id" --tags "Key=cloud-init,Value=true"
  aws ec2 create-tags --region "$REGION" --resources "$image_id" --tags "Key=version,Value=$version"
  aws ec2 create-tags --region "$REGION" --resources "$image_id" --tags "Key=build,Value=$build"
}

VERSION=$(basename $(remove_query_string "$OVA_URI") | sed -e 's/^aos_server_//' -e 's/.ova$//')
VERSION_WITHOUT_TAG=$(sed 's/[^0-9.].*$//' <<< "$VERSION")
setup_aws_stuff
fetch_image
new_volume
BEFORE=$(lsblk | grep disk | awk '{print $1}')
attach_volume
DISK=$(find_new_disk "$BEFORE")
time extract_vmdk "$DISK"

if check_version $VERSION_WITHOUT_TAG after 6.1.0; then
  aos_fixup
else
  aos_fixup_60x_and_earlier
fi

detach_volume
snapshot "$VOLUME_ID" "apstra $VERSION"
delete_volume
register_image

shutdown