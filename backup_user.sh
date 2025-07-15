#!/bin/bash
# backup_user.sh - A simple script to backup and restore users
# Usage: ./backup_user.sh [backup|restore] USER_ID

if [[ $# -ne 2 ]]; then
  echo "Usage: $0 [backup|restore] USER_ID"
  exit 1
fi

ACTION=$1
USER_ID=$2
DB_NAME="carbn"

echo "Action: $ACTION, User ID: $USER_ID"

# Backup user
if [[ "$ACTION" == "backup" ]]; then
  echo "Creating backup table if it doesn't exist..."
  psql $DB_NAME -c "CREATE TABLE IF NOT EXISTS backup_users (LIKE users INCLUDING ALL);"
  
  echo "Removing any previous backup for user $USER_ID..."
  psql $DB_NAME -c "DELETE FROM backup_users WHERE id = $USER_ID;"
  
  echo "Backing up user $USER_ID..."
  psql $DB_NAME -c "INSERT INTO backup_users SELECT * FROM users WHERE id = $USER_ID;"
  
  echo "Backup complete for user $USER_ID."

# Restore user
elif [[ "$ACTION" == "restore" ]]; then
  echo "Checking if backup exists for user $USER_ID..."
  USER_EXISTS=$(psql -t $DB_NAME -c "SELECT COUNT(*) FROM backup_users WHERE id = $USER_ID;")
  
  if [[ $USER_EXISTS -eq 0 ]]; then
    echo "No backup found for user $USER_ID!"
    exit 1
  fi
  
  echo "Checking if user $USER_ID currently exists..."
  USER_CURRENT_EXISTS=$(psql -t $DB_NAME -c "SELECT COUNT(*) FROM users WHERE id = $USER_ID;")
  
  if [[ $USER_CURRENT_EXISTS -eq 1 ]]; then
    echo "User $USER_ID exists. Updating fields instead of inserting..."
    
    # Update all fields one by one to maintain constraints
    psql $DB_NAME -c "
      UPDATE users 
      SET 
        email = b.email,
        auth_provider = b.auth_provider,
        auth_provider_id = b.auth_provider_id,
        full_name = b.full_name,
        display_name = b.display_name,
        followers_count = b.followers_count,
        profile_picture = b.profile_picture,
        created_at = b.created_at,
        updated_at = b.updated_at,
        is_private = b.is_private,
        currency = b.currency,
        last_login = b.last_login,
        is_private_email = b.is_private_email
      FROM backup_users b
      WHERE users.id = $USER_ID AND b.id = $USER_ID;
    "
  else
    echo "User $USER_ID doesn't exist. Inserting from backup..."
    psql $DB_NAME -c "INSERT INTO users SELECT * FROM backup_users WHERE id = $USER_ID;"
  fi
  
  echo "Restore complete for user $USER_ID."

else
  echo "Unknown action: $ACTION. Use 'backup' or 'restore'."
  exit 1
fi
