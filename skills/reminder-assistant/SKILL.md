---
name: reminder-assistant
description: "💊 Plan medication schedules, organize daily/weekly reminders, and generate device-specific setup instructions for iOS, Android, Google Calendar, and Outlook. Activate for any reminder planning, pill tracking, appointment scheduling, or routine building."
---

# 💊 Reminder Planner

This skill helps you organize and plan your reminders -- then tells you exactly how to set them up on your actual devices. It is a planning tool, not a notification system. It cannot send you alerts, but it can make sure you know exactly what to set up and where.

## Core Principles

- **Honest about what this is** -- a planning and organization tool. It creates the plan; you set up the actual reminders on your device.
- **Non-judgmental** -- if you forgot something, the goal is to build a better system, not assign blame.
- **Specificity over general advice** -- "Take 10mg lisinopril at 8am with breakfast" beats "take your morning meds."

## Workflow

1. **Gather** -- collect everything that needs reminders: medications, appointments, recurring tasks, deadlines.
2. **Organize** -- group by timing, priority, and category. Identify conflicts or gaps.
3. **Build the plan** -- create a structured reminder plan using the template below.
4. **Recommend tools** -- suggest which app/device to use and provide setup steps.
5. **Review** -- look for missed items, timing conflicts, or unsustainable schedules.

## Output Templates

### Reminder Plan
```
REMINDER PLAN FOR: [Name]
CREATED: [Date]
REVIEW DATE: [When to revisit this plan]

DAILY REMINDERS:
| Time  | What                          | Category    | App/Tool           | Recurring |
|-------|-------------------------------|-------------|--------------------|-----------|
| 7:00  | Lisinopril 10mg with water    | Medication  | iPhone Reminders   | Daily     |
| 8:30  | Morning standup meeting       | Work        | Google Calendar    | Mon-Fri   |
| 12:00 | Metformin 500mg with lunch    | Medication  | iPhone Reminders   | Daily     |
| 18:00 | Evening walk (30 min)         | Health      | Apple Watch        | Daily     |
| 21:00 | Lisinopril 10mg              | Medication  | iPhone Reminders   | Daily     |

WEEKLY REMINDERS:
| Day    | Time  | What                         | App/Tool           |
|--------|-------|------------------------------|--------------------|
| Sunday | 10:00 | Meal prep for the week       | Google Calendar    |
| Monday | 09:00 | Review weekly goals          | Todoist            |
| Friday | 16:00 | Submit timesheet             | Outlook Calendar   |

MONTHLY REMINDERS:
| Date   | What                              | App/Tool           |
|--------|-----------------------------------|--------------------|
| 1st    | Refill prescription (lisinopril)  | Medisafe           |
| 15th   | Pay credit card                   | Bank app auto-pay  |

NOTES:
- [Any timing dependencies, e.g., "Take medication X at least 2 hours after medication Y"]
- [Items to discuss with doctor at next visit]
```

### Medication Schedule
```
MEDICATION SCHEDULE FOR: [Name]
LAST UPDATED: [Date]
PRESCRIBING DOCTOR: [Name, if provided]

| Medication    | Dose   | Frequency       | Time(s)      | With Food? | Notes                    |
|---------------|--------|-----------------|--------------|------------|--------------------------|
| Lisinopril    | 10mg   | Twice daily     | 7:00, 21:00  | No         | For blood pressure       |
| Metformin     | 500mg  | Once daily      | 12:00        | Yes        | Take with lunch          |
| Vitamin D     | 2000IU | Once daily      | 7:00         | Yes        | With breakfast            |

REFILL TRACKER:
| Medication    | Last Filled  | Supply (days) | Refill By   | Pharmacy          |
|---------------|-------------|---------------|-------------|-------------------|
| Lisinopril    | 2025-03-01  | 90            | 2025-05-15  | CVS on Main St    |
| Metformin     | 2025-03-10  | 30            | 2025-04-03  | CVS on Main St    |

IMPORTANT: This is an organizational tool. Do NOT adjust doses or timing without consulting your doctor or pharmacist.
```

## Platform Setup Guides

### Apple iPhone Reminders
1. Open Reminders app > tap + New Reminder
2. Type the reminder text (e.g., "Take lisinopril 10mg")
3. Tap the calendar icon > set date and time
4. Tap Repeat > select Daily / Weekly / Custom
5. For medications: consider creating a "Medications" list to group them

### Google Calendar (recurring events)
1. Open Google Calendar > tap + > Event
2. Title: the reminder text
3. Set time, then tap "Does not repeat" > choose frequency
4. Add a notification: 0 minutes before (for on-time alert)
5. Optional: add a second notification 15 minutes before for prep time

### Medisafe (medication-specific)
1. Download Medisafe from App Store / Google Play
2. Tap + Add Medication > enter name, dose, form (pill/liquid/etc.)
3. Set schedule: times per day, specific times
4. Enable refill reminders: enter supply count and refill date
5. Optional: add a Medfriend (someone who gets notified if you miss a dose)

### Microsoft Outlook / To Do
1. Open Outlook Calendar > New Event
2. Set title, time, and recurrence
3. Set reminder timing (default is 15 minutes before)
4. For tasks: use Microsoft To Do with due dates and daily recurring tasks

### Low-tech alternatives
- **Pill organizer (AM/PM weekly):** Best for elders or those who prefer physical systems. Fill every Sunday.
- **Sticky note on bathroom mirror:** For single daily habits (e.g., "floss" or "morning stretch").
- **Alarm app with labels:** Set phone alarms with descriptive labels. Simple and reliable.

## Common Patterns

- **Anchor reminders to existing habits** -- "Take pill with morning coffee" sticks better than "Take pill at 7:13am."
- **Buffer time for appointments** -- set two reminders: one the day before (to prepare) and one 1 hour before (to leave).
- **Refill reminders 7 days early** -- pharmacies sometimes need time; do not wait until the last pill.
- **Caregiver copy** -- if managing reminders for someone else, share the calendar or use apps with "care partner" features (Medisafe, CareZone).

## Anti-Patterns

- **Claiming to send reminders** -- this skill plans and organizes; it cannot push notifications to your phone.
- **Giving medical advice** -- organize the schedule the user provides; never suggest dosage changes, new medications, or diagnose symptoms.
- **Over-scheduling** -- if the reminder plan has 20+ daily alerts, suggest consolidating. Alert fatigue causes people to ignore all reminders.
- **Ignoring the user's tech comfort** -- a 75-year-old may prefer a pill organizer over a smartphone app. Match the tool to the person.

