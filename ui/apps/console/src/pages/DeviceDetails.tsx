import { useEffect, useState } from "react";
import { useParams, useNavigate, useSearchParams } from "react-router-dom";
import Breadcrumb from "@/components/common/Breadcrumb";
import { LABEL_BASE } from "@/utils/styles";
import {
  TrashIcon,
  InformationCircleIcon,
  ComputerDesktopIcon,
  ClockIcon,
  CpuChipIcon,
  ChevronDoubleRightIcon,
  LockOpenIcon,
  LockClosedIcon,
} from "@heroicons/react/24/outline";
import { useDevice } from "../hooks/useDevice";
import { useDeviceActions } from "../hooks/useDeviceActions";
import { useUpdateDeviceSSH } from "../hooks/useDeviceMutations";
import { useHasPermission } from "../hooks/useHasPermission";
import { useNamespace } from "../hooks/useNamespaces";
import { useAuthStore } from "../stores/authStore";
import { useTerminalStore } from "../stores/terminalStore";
import DeviceActionsPortal from "./devices/DeviceActionsPortal";
import ConnectDrawer from "../components/ConnectDrawer";
import CopyButton from "../components/common/CopyButton";
import PlatformBadge from "../components/common/PlatformBadge";
import SettingToggle from "../components/common/SettingToggle";
import { formatDateFull, formatRelative } from "../utils/date";
import { buildSshid } from "../utils/sshid";
import RestrictedAction from "../components/common/RestrictedAction";
import PageLoader from "@/components/common/PageLoader";
import InfoItem from "./devices/InfoItem";
import TagsSection from "./devices/TagsSection";
import RenameSection from "./devices/RenameSection";
import CustomFieldsSection from "./devices/CustomFieldsSection";
import { Button, Card, IconButton } from "@shellhub/design-system/primitives";
import type { Device } from "../client";

/* ─── Shared styles ─── */
const VALUE = "text-sm text-text-primary font-medium mt-0.5";
type DeviceSSHSettings = NonNullable<Device["settings"]>;
type DeviceSSHSettingKey = keyof DeviceSSHSettings;

const DEVICE_SSH_SETTINGS: Array<{
  key: DeviceSSHSettingKey;
  title: string;
  description: string;
}> = [
  {
    key: "allow_password",
    title: "Allow Password Authentication",
    description: "Allow SSH connections using password for this device",
  },
  {
    key: "allow_public_key",
    title: "Allow Public Key Authentication",
    description: "Allow SSH connections using public key for this device",
  },
  {
    key: "allow_root",
    title: "Allow Root Login",
    description: "Allow SSH connections as root user for this device",
  },
  {
    key: "allow_empty_passwords",
    title: "Allow Empty Passwords",
    description: "Allow SSH connections with empty passwords for this device",
  },
  {
    key: "allow_tty",
    title: "Allow TTY Allocation",
    description: "Allow terminal (TTY) allocation for this device",
  },
  {
    key: "allow_tcp_forwarding",
    title: "Allow TCP Forwarding",
    description: "Allow TCP port forwarding for this device",
  },
  {
    key: "allow_web_endpoints",
    title: "Allow Web Endpoints",
    description: "Allow HTTP/HTTPS access via ShellHub proxy",
  },
  {
    key: "allow_sftp",
    title: "Allow SFTP",
    description: "Allow SFTP subsystem for this device",
  },
  {
    key: "allow_agent_forwarding",
    title: "Allow Agent Forwarding",
    description: "Allow SSH agent forwarding for this device",
  },
];

function ToggleStateIcon({ enabled }: { enabled: boolean }) {
  return enabled
    ? <LockOpenIcon className="w-4 h-4 text-accent-green" />
    : <LockClosedIcon className="w-4 h-4 text-accent-red" />;
}

/* ─── Page ─── */
export default function DeviceDetails() {
  const { uid } = useParams<{ uid: string }>();
  const navigate = useNavigate();
  const [searchParams] = useSearchParams();
  const { device, isLoading } = useDevice(uid ?? "");
  const updateSSH = useUpdateDeviceSSH();
  const canUpdateDeviceSettings = useHasPermission("device:update");
  const tenantId = useAuthStore((s) => s.tenant) ?? "";
  const { namespace: currentNamespace } = useNamespace(tenantId);
  const deviceSettings = device?.settings ?? {};
  const existingSession = useTerminalStore((s) =>
    s.sessions.find((sess) => sess.deviceUid === uid),
  );
  const restoreTerminal = useTerminalStore((s) => s.restore);
  const [connectOpen, setConnectOpen] = useState(false);
  const actionsController = useDeviceActions({
    onSuccess: (action) => {
      if (action === "remove") void navigate("/devices");
    },
  });

  const updateDeviceSetting = async (settings: Partial<DeviceSSHSettings>) => {
    if (!device) {
      return;
    }

    await updateSSH.mutateAsync({ path: { uid: device.uid }, body: settings });
  };

  // Auto-open connect drawer if ?connect=true (adjust during render)
  const shouldAutoConnect =
    searchParams.get("connect") === "true" &&
    device?.online &&
    !existingSession;

  const [autoConnectDone, setAutoConnectDone] = useState(false);
  if (shouldAutoConnect && !autoConnectDone) {
    setAutoConnectDone(true);
    setConnectOpen(true);
  }
  if (!shouldAutoConnect && autoConnectDone) {
    setAutoConnectDone(false);
  }

  // Restore existing terminal session (side effect only, no setState)
  useEffect(() => {
    if (
      searchParams.get("connect") === "true" &&
      device?.online &&
      existingSession
    ) {
      restoreTerminal(existingSession.id);
    }
  }, [searchParams, device, existingSession, restoreTerminal]);

  if (isLoading || !device) {
    return <PageLoader label="Loading device details" />;
  }

  const nsName = currentNamespace?.name ?? "";
  const sshid = nsName ? buildSshid(nsName, device.name) : device.uid;

  const tags: string[] = Array.isArray(device.tags)
    ? device.tags.map((t) =>
        typeof t === "object" && t !== null && "name" in t ? t.name : String(t),
      )
    : [];

  return (
    <div className="animate-fade-in">
      <Breadcrumb
        items={[{ label: "Devices", to: "/devices" }, { label: device.name }]}
      />

      {/* Header */}
      <div className="flex flex-col sm:flex-row sm:items-start justify-between gap-4 mb-8">
        <div className="flex items-start gap-4">
          {/* Device icon with status */}
          <div className="relative shrink-0">
            <div className="w-14 h-14 rounded-xl bg-primary/10 border border-primary/20 flex items-center justify-center">
              <CpuChipIcon className="w-7 h-7 text-primary" />
            </div>
            <span
              className={`absolute -bottom-1 -right-1 w-4 h-4 rounded-full border-2 border-background ${
                device.online
                  ? "bg-accent-green shadow-[0_0_8px_rgba(130,165,104,0.5)]"
                  : "bg-text-muted/40"
              }`}
            />
          </div>

          <div>
            <RenameSection uid={device.uid} currentName={device.name} />
            <div className="flex items-center gap-2 mt-1.5">
              <span
                className={`inline-flex items-center gap-1 px-2 py-0.5 text-2xs font-semibold rounded-md ${
                  device.online
                    ? "bg-accent-green/10 text-accent-green border border-accent-green/20"
                    : "bg-text-muted/10 text-text-muted border border-border"
                }`}
              >
                <span
                  className={`w-1.5 h-1.5 rounded-full ${device.online ? "bg-accent-green" : "bg-text-muted/60"}`}
                />
                {device.online ? "Online" : "Offline"}
              </span>
              <span
                className={`inline-flex items-center px-2 py-0.5 text-2xs font-medium rounded-md ${
                  device.status === "accepted"
                    ? "bg-accent-green/10 text-accent-green"
                    : device.status === "pending"
                      ? "bg-accent-yellow/10 text-accent-yellow"
                      : "bg-accent-red/10 text-accent-red"
                }`}
              >
                {device.status.charAt(0).toUpperCase() + device.status.slice(1)}
              </span>
            </div>
          </div>
        </div>

        {/* Actions */}
        <div className="flex items-center gap-2 shrink-0">
          {device.status === "accepted" && (
            <>
              <RestrictedAction action="device:connect">
                <Button
                  variant="success"
                  onClick={() => {
                    if (existingSession) {
                      restoreTerminal(existingSession.id);
                    } else {
                      setConnectOpen(true);
                    }
                  }}
                  disabled={!device.online}
                  icon={
                    <ChevronDoubleRightIcon
                      className="w-4 h-4"
                      strokeWidth={2}
                    />
                  }
                >
                  Connect
                </Button>
              </RestrictedAction>
              <RestrictedAction action="device:remove">
                <IconButton
                  variant="danger"
                  size="lg"
                  type="button"
                  title="Delete device"
                  aria-label="Delete device"
                  className="border border-border"
                  onClick={() => actionsController.requestAction(device, "remove")}
                >
                  <TrashIcon className="w-4 h-4" />
                </IconButton>
              </RestrictedAction>
            </>
          )}
          {device.status === "pending" && (
            <>
              <RestrictedAction action="device:accept">
                <Button
                  variant="success"
                  onClick={() => actionsController.requestAction(device, "accept")}
                >
                  Accept
                </Button>
              </RestrictedAction>
              <RestrictedAction action="device:reject">
                <Button
                  variant="warning"
                  onClick={() => actionsController.requestAction(device, "reject")}
                >
                  Reject
                </Button>
              </RestrictedAction>
            </>
          )}
          {device.status === "rejected" && (
            <>
              <RestrictedAction action="device:accept">
                <Button
                  variant="success"
                  onClick={() => actionsController.requestAction(device, "accept")}
                >
                  Accept
                </Button>
              </RestrictedAction>
              <RestrictedAction action="device:remove">
                <Button
                  variant="destructive"
                  onClick={() => actionsController.requestAction(device, "remove")}
                >
                  Remove
                </Button>
              </RestrictedAction>
            </>
          )}
        </div>
      </div>

      {/* SSHID Banner */}
      {device.status === "accepted" && (
        <Card className="p-4 mb-6 flex items-center justify-between gap-4">
          <div>
            <p className={LABEL_BASE}>SSHID</p>
            <code className="text-sm font-mono text-accent-cyan mt-0.5 block">
              {sshid}
            </code>
          </div>
          <CopyButton text={sshid} />
        </Card>
      )}

      {/* Info Grid */}
      <div className="grid grid-cols-1 md:grid-cols-2 lg:grid-cols-3 gap-6 mb-8">
        <Card className="p-5 space-y-4">
          <h3 className="text-xs font-semibold text-text-primary flex items-center gap-2">
            <InformationCircleIcon className="w-4 h-4 text-primary" />
            Identity
          </h3>
          <dl className="space-y-3">
            <InfoItem
              label="UID"
              value={device.uid}
              mono
              copyable
              truncate={8}
            />
            <InfoItem
              label="MAC Address"
              value={device.identity?.mac ?? ""}
              mono
              copyable
            />
            <InfoItem
              label="Remote Address"
              value={device.remote_addr ?? ""}
              mono
            />
          </dl>
        </Card>

        <Card className="p-5 space-y-4">
          <h3 className="text-xs font-semibold text-text-primary flex items-center gap-2">
            <ComputerDesktopIcon className="w-4 h-4 text-primary" />
            System
          </h3>
          <dl className="space-y-3">
            <InfoItem
              label="Operating System"
              value={device.info?.pretty_name ?? ""}
            />
            <InfoItem
              label="Architecture"
              value={device.info?.arch ?? ""}
              mono
            />
            <div>
              <dt className={LABEL_BASE}>Platform</dt>
              <dd className="mt-1">
                {device.info?.platform ? (
                  <PlatformBadge platform={device.info.platform} />
                ) : (
                  <span className="text-sm text-text-muted">—</span>
                )}
              </dd>
            </div>
            <InfoItem
              label="Agent Version"
              value={device.info?.version ?? ""}
              mono
            />
          </dl>
        </Card>

        <Card className="p-5 space-y-4">
          <h3 className="text-xs font-semibold text-text-primary flex items-center gap-2">
            <ClockIcon className="w-4 h-4 text-primary" />
            Timeline
          </h3>
          <dl className="space-y-3">
            <div>
              <dt className={LABEL_BASE}>Created</dt>
              <dd className={VALUE}>{formatDateFull(device.created_at)}</dd>
            </div>
            <div>
              <dt className={LABEL_BASE}>Last Seen</dt>
              <dd className="flex items-center gap-2 mt-0.5">
                <span className="text-sm text-text-primary font-medium">
                  {formatRelative(device.last_seen)}
                </span>
                <span className="text-2xs text-text-muted">
                  {formatDateFull(device.last_seen)}
                </span>
              </dd>
            </div>
            <div>
              <dt className={LABEL_BASE}>Status Updated</dt>
              <dd className={VALUE}>
                {formatDateFull(device.status_update_at ?? "")}
              </dd>
            </div>
          </dl>
        </Card>
      </div>

      {/* Tags + Custom Fields */}
      <div className="grid grid-cols-1 md:grid-cols-2 gap-6 mb-6">
        <Card className="p-5">
          <TagsSection uid={device.uid} tags={tags} />
        </Card>
        <Card className="p-5">
          <CustomFieldsSection
            uid={device.uid}
            customFields={device.custom_fields ?? {}}
          />
        </Card>
      </div>

      {/* Settings */}
      <Card className="p-5 mb-6">
        <h3 className="text-xs font-semibold text-text-primary flex items-center gap-2 mb-4">
          <LockClosedIcon className="w-4 h-4 text-primary" />
          Settings
        </h3>
        <div className="divide-y divide-border -mx-2">
          {DEVICE_SSH_SETTINGS.map((setting) => {
            const enabled = deviceSettings[setting.key] ?? true;

            return (
              <div key={setting.key} className="flex items-center justify-between gap-6 px-2 py-3">
                <div className="flex items-start gap-3 min-w-0 flex-1">
                  <span className="w-8 h-8 rounded-lg bg-hover-medium border border-border flex items-center justify-center text-text-muted shrink-0 mt-0.5">
                    <ToggleStateIcon enabled={enabled} />
                  </span>
                  <div className="min-w-0">
                    <p className="text-sm font-medium text-text-primary">
                      {setting.title}
                    </p>
                    <p className="text-2xs text-text-muted mt-0.5 leading-relaxed">
                      {setting.description}
                    </p>
                  </div>
                </div>
                <div className="shrink-0">
                  <SettingToggle
                    checked={enabled}
                    tone="success"
                    disabled={!canUpdateDeviceSettings || updateSSH.isPending}
                    onChange={(checked) => {
                      return updateDeviceSetting({ [setting.key]: checked });
                    }}
                  />
                </div>
              </div>
            );
          })}
        </div>
      </Card>

      {/* Connect Drawer */}
      <ConnectDrawer
        open={connectOpen}
        onClose={() => setConnectOpen(false)}
        deviceUid={device.uid}
        deviceName={device.name}
        sshid={sshid}
      />

      {/* Action Portal (accept/reject/remove for pending/rejected devices) */}
      <DeviceActionsPortal controller={actionsController} />
    </div>
  );
}
