import { useState } from "react";
import { defaultMessageComposer, post, type MessagePayload } from "../api.js";
import { Modal } from "./Modal.js";
import { Button, FieldLabel, Input } from "./ui.js";

type Target = { type: "player"; player: string } | { type: "all" };

type Props = {
  open: boolean;
  target: Target | null;
  onClose: () => void;
  onSent: (msg: string) => void;
};

export function MessageComposerModal({ open, target, onClose, onSent }: Props) {
  const [draft, setDraft] = useState(() => defaultMessageComposer());

  const send = async () => {
    if (!target || !draft.text.trim()) return;
    const payload: MessagePayload = {
      message: draft.text,
      duration: draft.duration,
      x: draft.x,
      y: draft.y,
      fontsize: draft.fontsize,
      fg: draft.fg,
      bg: draft.bg,
    };
    const path = target.type === "player" ? "/api/message_player" : "/api/message_all";
    const body = target.type === "player" ? { ...payload, player: target.player } : payload;
    const res = await post(path, body);
    if (res.ok) {
      onSent(
        target.type === "player"
          ? `message sent to ${target.player}`
          : "message sent to all players"
      );
      setDraft((d) => ({
        ...defaultMessageComposer(),
        duration: d.duration,
        x: d.x,
        y: d.y,
        fontsize: d.fontsize,
        fg: d.fg,
        bg: d.bg,
      }));
    } else {
      onSent("message send failed");
    }
  };

  const title = target?.type === "player" ? `Message: ${target.player}` : "Message all players";

  return (
    <Modal
      open={open}
      title={title}
      onClose={onClose}
      footer={
        <>
          <Button variant="ghost" onClick={() => setDraft(defaultMessageComposer())}>
            Reset
          </Button>
          <Button variant="ghost" onClick={onClose}>
            Cancel
          </Button>
          <Button variant="primary" disabled={!draft.text.trim()} onClick={() => void send()}>
            Send
          </Button>
        </>
      }
    >
      <div className="space-y-3">
        <div>
          <FieldLabel>Message</FieldLabel>
          <textarea
            className="mt-1 w-full rounded-lg border border-slate-700 bg-slate-950/80 px-2.5 py-1.5 text-sm text-slate-100"
            rows={3}
            value={draft.text}
            onChange={(e) => setDraft((d) => ({ ...d, text: e.target.value }))}
            placeholder="Enter your message…"
          />
        </div>
        <div className="grid grid-cols-2 gap-3">
          <div>
            <FieldLabel>Duration (sec)</FieldLabel>
            <Input
              type="number"
              min={1}
              max={60}
              value={draft.duration}
              onChange={(e) => setDraft((d) => ({ ...d, duration: +e.target.value }))}
            />
          </div>
          <div>
            <FieldLabel>Font size</FieldLabel>
            <Input
              type="number"
              min={8}
              max={48}
              value={draft.fontsize}
              onChange={(e) => setDraft((d) => ({ ...d, fontsize: +e.target.value }))}
            />
          </div>
          <div>
            <FieldLabel>X</FieldLabel>
            <Input
              type="number"
              value={draft.x}
              onChange={(e) => setDraft((d) => ({ ...d, x: +e.target.value }))}
            />
          </div>
          <div>
            <FieldLabel>Y</FieldLabel>
            <Input
              type="number"
              value={draft.y}
              onChange={(e) => setDraft((d) => ({ ...d, y: +e.target.value }))}
            />
          </div>
          <div>
            <FieldLabel>Text color</FieldLabel>
            <Input
              type="color"
              value={draft.fg}
              onChange={(e) => setDraft((d) => ({ ...d, fg: e.target.value }))}
              className="h-9 p-1"
            />
          </div>
          <div>
            <FieldLabel>Background</FieldLabel>
            <Input
              type="color"
              value={draft.bg}
              onChange={(e) => setDraft((d) => ({ ...d, bg: e.target.value }))}
              className="h-9 p-1"
            />
          </div>
        </div>
      </div>
    </Modal>
  );
}
