package ru.quicdiver.client;

// The core keeps one log of what it does, shared with the desktop client, and
// writes it in English. Rather than fork that text per platform, the phone
// recognises what it knows and says it in its own words.
public final class Words {

    private Words() {
    }

    public static String of(String raw) {
        if (raw == null || raw.isEmpty()) {
            return "";
        }

        if (raw.startsWith("Subscription checked")) {
            return "Подписка обновлена";
        }
        if (raw.startsWith("Subscription imported")) {
            return "Подписка принята";
        }
        if (raw.startsWith("Connected through")) {
            return "Подключено";
        }
        if (raw.startsWith("Disconnected")) {
            return "Отключено";
        }
        if (raw.startsWith("No entrypoint answered")) {
            return "Ни один узел не ответил";
        }
        if (raw.startsWith("No node answered this link")) {
            return "По этой ссылке пока никто не ответил";
        }
        if (raw.startsWith("Could not bring the tunnel up")) {
            return "Туннель не поднялся";
        }
        if (raw.startsWith("Exit nodes were refused")) {
            return "Выходные узлы не разрешены подписке";
        }
        if (raw.startsWith("Exit nodes are no longer allowed")) {
            return "Выходные узлы больше не разрешены";
        }
        if (raw.startsWith("This subscription is no longer valid")) {
            return "Подписка недействительна";
        }
        if (raw.startsWith("This client has been disabled")) {
            return "Клиент отключён администратором";
        }
        if (raw.startsWith("This subscription has expired")) {
            return "Срок подписки истёк";
        }

        return raw;
    }
}
