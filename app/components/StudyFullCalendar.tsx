"use client";

import FullCalendar from "@fullcalendar/react";
import dayGridPlugin from "@fullcalendar/daygrid";
import interactionPlugin from "@fullcalendar/interaction";
import type { EventClickArg, EventDropArg } from "@fullcalendar/core";
import type { DateClickArg } from "@fullcalendar/interaction";

export type StudyCalendarEvent = {
  id: string;
  title: string;
  date: string; // "YYYY-MM-DD"
  color?: string; // 科目カラー（学習予定用）
  done?: boolean; // 完了なら打ち消し線＋淡色
};

type Props = {
  events: StudyCalendarEvent[];
  // 受験日程（読み取り専用・別色・ドラッグ/編集不可）
  examEvents: StudyCalendarEvent[];
  onEventClick: (id: string) => void;
  onDateClick: (date: string) => void;
  onEventDrop: (id: string, newDate: string) => void;
  // ドラッグでの日移動を許可するか（デモは閲覧専用のため false）
  editable?: boolean;
};

export default function StudyFullCalendar({
  events,
  examEvents,
  onEventClick,
  onDateClick,
  onEventDrop,
  editable = true,
}: Props) {
  return (
    <FullCalendar
      plugins={[dayGridPlugin, interactionPlugin]}
      initialView="dayGridMonth"
      locale="ja"
      height="70vh"
      headerToolbar={{
        left: "prev,next today",
        center: "title",
        right: "",
      }}
      buttonText={{ today: "今日" }}
      events={[
        ...events.map((e) => ({
          id: e.id,
          title: e.title,
          start: e.date,
          allDay: true,
          editable,
          color: e.color,
          classNames: e.done ? ["study-done"] : [],
        })),
        ...examEvents.map((e) => ({
          id: e.id,
          title: e.title,
          start: e.date,
          allDay: true,
          editable: false,
          color: "#dc2626", // 受験日は赤系で強調
        })),
      ]}
      editable={editable}
      eventClick={(arg: EventClickArg) => onEventClick(arg.event.id)}
      dateClick={(arg: DateClickArg) => onDateClick(arg.dateStr)}
      eventDrop={(arg: EventDropArg) => {
        const newDate = arg.event.startStr.slice(0, 10);
        onEventDrop(arg.event.id, newDate);
      }}
    />
  );
}
