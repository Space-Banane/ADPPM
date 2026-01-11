# 🎨 Active Directory Profile Picture Management

**ADPPM** is a lightweight Go-based tool designed for SysAdmins to efficiently manage and update profile pictures in Active Directory.

---

## 💡 Why?

I couldn't find a proper tool that works from anywhere with a simple, lightweight UI—so I built one myself.

## ✨ Features

- 👥 View and manage user profiles
- 📸 Upload new profile pictures
- ⭕ Auto-round profile pictures for a polished look

---

## 🚀 Getting Started

0. Clone the repository:

   ```bash
   git clone https://github.com/Space-Banane/ADPPM.git
   cd ADPPM
   ```
1. Install dependencies:

   ```bash
   go mod tidy
   ```
2. Build the project:

   ```bash
   go build .
   ```
3. Run the executable
4. Open your browser and navigate to `http://localhost:8080`
5. Follow the on-screen instructions to manage profile pictures

### 🎁 Bonus

Configure `config.json` to enable authentication and remote access (hosts the web interface on all network interfaces)

---

## 📋 Requirements

- Go 1.18 or higher
- Windows machine with Active Directory installed

---

## 🙏 Thanks!