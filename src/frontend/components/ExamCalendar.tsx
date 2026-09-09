"use client";

import { useCalendarApp, ScheduleXCalendar } from "@schedule-x/react";
import { createViewMonthGrid } from "@schedule-x/calendar";
import "temporal-polyfill/global";
import "@schedule-x/theme-default/dist/index.css";

type ExamEvent = {
  id: string;
  title: string;
  date: string;
};

const ExamCalendar = ({ events }: { events: ExamEvent[] }) => {
  const calendar = useCalendarApp({
    views: [createViewMonthGrid()],
    events: events.map((event) => ({
      id: event.id,
      title: event.title,
      start: Temporal.PlainDate.from(event.date),
      end: Temporal.PlainDate.from(event.date),
    })),
  });

  return <ScheduleXCalendar calendarApp={calendar} />;
};

export default ExamCalendar;