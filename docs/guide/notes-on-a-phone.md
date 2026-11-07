# Notes on a phone

## The iPhone Shortcut

One Shortcut, "pcom note", stores a note in pcom from anywhere on the iPhone. Run from the Action button
(or Back Tap, a Control Center control or Siri) it asks nothing and starts recording; stop the recording
and it is sent. Run from the share sheet it sends what you shared: text, a link or photos. A second
Shortcut, "Quote from photo", takes a photo of a book page, reads its text on the phone and sends both as
a quote.

### Before you start

In pcom, open Settings, and under **API keys** create a key named after the phone with access **Notes
only**. Copy it: it is shown only once. A notes key can add and read your notes and nothing else, so a
phone that holds it can't change or delete anything in pcom.

### Build "pcom note"

In the Shortcuts app, tap **+** and name the Shortcut "pcom note". In its details (the **ⓘ** button),
turn on **Show in Share Sheet** and choose the types **Text**, **URLs** and **Images**. Then add these
actions in order:

1. **Receive** *Text, URLs and Images* **input from** *Share Sheet*; **If there's no input:** *Continue*.
2. **Date**: *Current Date*. Then **Format Date**: format *ISO 8601*, *Include ISO 8601 Time* on.
3. **If** *Shortcut Input* **has no value**: the run from the Action button.
   1. **Record Audio**: *Audio Quality* Normal, *Start Recording* **Immediately**, *Finish Recording* **On Tap**.
   2. **Get Contents of URL**: URL `https://<your pcom>/api/v1/notes`, **Method** POST, **Headers**
      `Authorization` = `Bearer <your key>`, **Request Body** Form with the fields: `kind` (Text) =
      `note`, `captured_at` (Text) = *Formatted Date*, `audio` (File) = *Recorded Audio*.
4. **Otherwise**: the run from the share sheet.
   1. **Get Images from** *Shortcut Input*.
   2. **If** *Images* **has any value**:
      1. **Convert** *Images* **to** JPEG (an iPhone photo is HEIC, which pcom doesn't read).
      2. **Get Contents of URL** as in step 3.2, with the Form fields `kind` = `share`,
         `captured_at` = *Formatted Date*, and `image` (File) = *Converted Image*. Shortcuts sends one
         `image` file for each photo.
   3. **Otherwise**: **Get Contents of URL** as in step 3.2, with the Form fields `kind` = `share`,
      `captured_at` = *Formatted Date* and `text` (Text) = *Shortcut Input*. A link arrives as text and
      is stored as a bare link.
   4. **End If**.
5. **End If**. Then **Show Notification** with *Contents of URL*, if you want to see what pcom answered;
   when pcom refuses a capture, the answer says why.

To start it from the **Action button**: Settings → Action Button → Shortcut → "pcom note". For **Back
Tap**: Settings → Accessibility → Touch → Back Tap. It also works from Siri ("pcom note") and as a
Control Center or lock-screen control.

### Build "Quote from photo"

1. **Take Photo**: *Show Camera Preview* on.
2. **Convert** *Photo* **to** JPEG.
3. **Extract Text from** *Converted Image* (done on the phone).
4. **Date**: *Current Date*, then **Format Date** as ISO 8601, as above.
5. **Get Contents of URL**: the same URL, method and header, **Request Body** Form with `kind` = `quote`,
   `captured_at` = *Formatted Date*, `text` = *Text from Image* and `image` (File) = *Converted Image*.

The text is stored as a quote. If your previous quote came from a book within the focus idle time, the
new one takes that book as its source; otherwise it waits in the Inbox, where you give it a source with
one tap. To quote from the Kindle app, which has no Share for a highlight, copy the passage and run a copy
of this Shortcut whose step 1 to 3 are **Get Clipboard**.

### What the request looks like

The exact fields each run sends are listed in [the API documentation](../api.md), under "The requests of
the iPhone Shortcut".
