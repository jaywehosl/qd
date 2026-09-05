package ru.quicdiver.client;

import android.app.Activity;
import android.content.ClipData;
import android.content.ClipboardManager;
import android.util.TypedValue;
import android.view.Gravity;
import android.view.View;
import android.widget.EditText;
import android.widget.LinearLayout;
import android.widget.TextView;

import qdmobile.Client;

public class ImportPage {

    private final Activity host;
    private final Skin skin;
    private final Runnable onDone;

    private View made;
    private EditText field;
    private TextView trouble;

    public ImportPage(Activity host, Skin skin, Runnable onDone) {
        this.host = host;
        this.skin = skin;
        this.onDone = onDone;
    }

    public View build() {
        if (made != null) {
            return made;
        }

        LinearLayout root = skin.column();
        root.setGravity(Gravity.CENTER);
        root.setBackground(skin.backdrop());
        root.setPadding(skin.dp(24), skin.dp(24), skin.dp(24), skin.dp(24));

        TextView head = skin.label("qd", skin.good, 28);
        head.setGravity(Gravity.CENTER);
        root.addView(head);

        TextView note = skin.label("Вставь ссылку подписки.", skin.muted, 15);
        note.setGravity(Gravity.CENTER);
        note.setPadding(0, skin.dp(12), 0, skin.dp(28));
        root.addView(note);

        LinearLayout box = skin.card();

        field = new EditText(host);
        field.setHint("qd://…");
        field.setTextColor(skin.text);
        field.setTextSize(TypedValue.COMPLEX_UNIT_SP, 15);
        field.setMinLines(2);
        field.setGravity(Gravity.TOP | Gravity.START);
        box.addView(field);

        TextView apply = button(box, "Применить", skin.good);
        apply.setOnClickListener(new View.OnClickListener() {
            @Override
            public void onClick(View v) {
                adopt(field.getText().toString());
            }
        });

        TextView paste = button(box, "Вставить из буфера и применить", skin.muted);
        paste.setOnClickListener(new View.OnClickListener() {
            @Override
            public void onClick(View v) {
                fromClipboard();
            }
        });

        trouble = skin.note("");
        trouble.setGravity(Gravity.CENTER);
        box.addView(trouble);

        root.addView(box, new LinearLayout.LayoutParams(
                LinearLayout.LayoutParams.MATCH_PARENT, LinearLayout.LayoutParams.WRAP_CONTENT));

        made = root;
        return root;
    }

    private void say(String text, boolean bad) {
        if (trouble == null) {
            return;
        }
        trouble.setText(text == null ? "" : text);
        trouble.setTextColor(bad ? 0xFFCF4444 : skin.muted);
    }

    private TextView button(LinearLayout parent, String caption, int tint) {
        TextView view = skin.label(caption, tint, 16);
        view.setGravity(Gravity.CENTER);
        view.setPadding(0, skin.dp(16), 0, skin.dp(4));
        parent.addView(view);
        return view;
    }

    private void fromClipboard() {
        ClipboardManager board = host.getSystemService(ClipboardManager.class);
        ClipData clip = board == null ? null : board.getPrimaryClip();
        if (clip == null || clip.getItemCount() == 0) {
            say("Буфер пуст", true);
            return;
        }

        CharSequence held = clip.getItemAt(0).coerceToText(host);
        String uri = held == null ? "" : held.toString().trim();
        if (uri.isEmpty()) {
            say("Буфер пуст", true);
            return;
        }

        field.setText(uri);
        adopt(uri);
    }

    public void adopt(String raw) {
        final String uri = raw == null ? "" : raw.trim();
        if (uri.isEmpty()) {
            say("Ссылка не введена", true);
            return;
        }

        new Thread(new Runnable() {
            @Override
            public void run() {
                String problem = null;
                String label = "";
                try {
                    Client client = Core.client(host);
                    client.import_(uri);
                    label = client.label();
                } catch (Exception e) {
                    problem = e.getMessage();
                }

                final String said = problem;
                final String name = label;
                host.runOnUiThread(new Runnable() {
                    @Override
                    public void run() {
                        if (said != null) {
                            say(said, true);
                            return;
                        }

                        onDone.run();
                    }
                });
            }
        }).start();
    }
}
