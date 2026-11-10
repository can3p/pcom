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
      1. **Get Contents of URL** as in step 3.2, with the Form fields `kind` = `share`,
         `captured_at` = *Formatted Date*, and `image` (File) = *Images*. Shortcuts sends one
         `image` file for each photo; an iPhone photo (HEIC) is converted to JPEG by pcom.
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
2. **Extract Text from** *Photo* (done on the phone).
3. **Date**: *Current Date*, then **Format Date** as ISO 8601, as above.
4. **Get Contents of URL**: the same URL, method and header, **Request Body** Form with `kind` = `quote`,
   `captured_at` = *Formatted Date*, `text` = *Text from Image* and `image` (File) = *Photo*.

The text is stored as a quote. If your previous note with a source was captured within the last twelve
hours (by default), the new one takes that source; otherwise choose its source on the note's edit page. To quote from the Kindle app, which has no Share for a highlight, copy the passage and run a copy
of this Shortcut whose step 1 and 2 are **Get Clipboard**.

### What the request looks like

The exact fields each run sends are listed in [the API documentation](../api.md), under "The requests of
the iPhone Shortcut".

## Quotes from books

Quotes need no retyping: the phone's own reader or its text recognition has the words already, and pcom stores
what you send. A quote keeps its source; a passage with no source can be given one on its edit page.

| Where the passage is | iPhone | Android |
|---|---|---|
| Kindle app | Select the passage, then **Copy** (the Kindle app for iPhone has no Share for a highlight), then run the copy of "pcom note" described above, whose input is the clipboard | Select the passage, then **Share**, then **pcom** |
| Paper book | Camera, then Live Text, select the passage, **Share**, then **pcom note**; or run "Quote from photo" | Google Lens or the camera's text selection, select, **Share**, then **pcom** |

Kindle and Live Text share only the selected words, with no title or location, so each passage arrives
bare. The source is the one of your previous quote while that is recent, which means the quotes of one reading
session join one book without any setup.

## On Android

pcom runs as an installed web app, which adds the share target and the app shortcuts.

1. Open pcom in Chrome and sign in.
2. Open the Chrome menu and choose **Install app** (or **Add to Home screen**). The app opens on the capture
   page and has its own icon.
3. Long-press the icon for two shortcuts, **Dictate** and **Photo**, which start that capture at once.

After installing, **Share** from any app to pcom: text and passages from a reader, links, photos, and an audio
file such as a voice memo. Text is stored as a quote, a lone link as a link, and a photo or recording as a note of
its own. The share needs the installed app: a browser tab doesn't receive shares.

A share carries only its text, link, photos or audio. Tags, a source and a time can't be set by a share: the tags
are the `#words` in its text, and the note is dated when it is received. A share is sent to pcom at once. If the
network fails, the share is kept on the phone and uploaded when the connection returns.

On Android, dictation runs in the browser with the screen on, so keep the phone unlocked while you record.

## Testing from a phone

A phone can't record through a plain-HTTP address, since browsers give the microphone and camera only to HTTPS
and `localhost`. To try the capture page from a phone while developing, serve pcom over HTTPS through a tunnel to
the local server. [Running pcom locally](../running.md) lists the speech-to-text settings the local stack uses.
